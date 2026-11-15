package model

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm/clause"
)

// 跨节点共享渠道 RPM：多节点（香港 / 美国）与蓝绿双容器各自的内存计数经共用数据库定时交换，
// 让编排页展示与水位线判定都按全站口径。
//
// 设计要点（稳定性优先）：
//   - 请求路径零新增 IO：选中渠道仍只在内存 +1；读取时多加一次无锁的原子指针读。
//   - 后台每 channelRpmSharedSyncInterval 上报本进程快照（一个进程一行，整份快照放 payload），
//     并读回其他进程的行。
//   - 存活判定不依赖各机器的墙上时钟：只看「本机观测到对方 seq 推进」的时刻，
//     超过 channelRpmSharedPeerTTL 未推进即不再计入（节点宕机、蓝绿旧容器退场自动收敛）。
//   - 数据库出问题时自动退回「只算本进程」，即未开启共享时的行为。
//
// 详见 docs/channel/cascade-failover.md

const (
	// channelRpmSharedSyncInterval 上报与拉取的间隔
	channelRpmSharedSyncInterval = 3 * time.Second
	// channelRpmSharedTimeout 单轮同步的数据库超时
	channelRpmSharedTimeout = 2 * time.Second
	// channelRpmSharedPeerTTL 对方超过该时长没有新上报即不再计入
	channelRpmSharedPeerTTL = 15 * time.Second
	// channelRpmSharedRowRetention 对方超过该时长没有新上报即删掉它留在库里的行
	channelRpmSharedRowRetention = 10 * time.Minute
	// channelRpmSharedLogInterval 同步失败日志的最小间隔，避免数据库故障时刷屏
	channelRpmSharedLogInterval = time.Minute
)

// ChannelRpmReport 一个进程上报的渠道 RPM 快照
type ChannelRpmReport struct {
	// NodeId 进程标识：节点名 + 启动时随机后缀（同机蓝绿两个容器互不覆盖）
	NodeId string `json:"node_id" gorm:"primaryKey;type:varchar(128)"`
	// Seq 每次上报自增，对方据此判断本进程是否仍在上报
	Seq int64 `json:"seq"`
	// Payload 分组 → 渠道 → 近 60 秒 RPM 的 JSON
	Payload string `json:"payload" gorm:"type:text"`
	// ReportedAt 上报时刻（unix 秒），仅供人工排查，不参与存活判定
	ReportedAt int64 `json:"reported_at"`
}

func (ChannelRpmReport) TableName() string {
	return "channel_rpm_reports"
}

// channelRpmPeer 本机对另一个进程的观测状态
type channelRpmPeer struct {
	seq       int64
	changedAt time.Time // 本机最近一次观测到 seq 变化（首次见到也算）的时刻
	advanced  bool      // 是否亲眼见过 seq 推进；首次见到的行可能是早已退场进程的遗留，不计入
	rpm       map[string]map[int]int64
}

var (
	channelRpmSharedNodeId = fmt.Sprintf("%s-%s", common.NodeName, common.GetRandomString(6))

	// channelRpmRemote 其他存活进程的 RPM 合计（不可变快照，整体替换）；nil = 无远端数据
	channelRpmRemote atomic.Pointer[map[string]map[int]int64]
	// channelRpmRemotePeers 当前计入的其他进程数，编排页展示用
	channelRpmRemotePeers atomic.Int64

	// 以下状态只由同步循环（及测试）访问，用一把锁兜住
	channelRpmSharedLock        sync.Mutex
	channelRpmSharedPeers       = make(map[string]*channelRpmPeer)
	channelRpmSharedSeq         int64
	channelRpmSharedLastEmpty   bool
	channelRpmSharedLastLogTime time.Time
)

// getRemoteChannelRpm 其他进程上报的该「分组 + 渠道」RPM 合计
func getRemoteChannelRpm(group string, channelId int) int64 {
	remote := channelRpmRemote.Load()
	if remote == nil {
		return 0
	}
	return (*remote)[group][channelId]
}

// GetChannelRpmSharedPeerCount 当前计入全站 RPM 的其他进程数（未开启共享时恒为 0）
func GetChannelRpmSharedPeerCount() int {
	return int(channelRpmRemotePeers.Load())
}

// StartChannelRpmSharedSyncLoop 周期性上报本进程 RPM 并拉取其他进程的上报
func StartChannelRpmSharedSyncLoop() {
	for {
		time.Sleep(channelRpmSharedSyncInterval)
		syncChannelRpmShared(time.Now())
	}
}

func syncChannelRpmShared(now time.Time) {
	defer func() {
		// 同步只是锦上添花，任何意外都不能带崩进程
		if r := recover(); r != nil {
			common.SysError(fmt.Sprintf("shared channel rpm sync panic: %v", r))
		}
	}()

	channelRpmSharedLock.Lock()
	defer channelRpmSharedLock.Unlock()

	if !operation_setting.GetCascadeSetting().SharedRpmEnabled {
		// 关闭即回到只算本进程：清掉远端数据与观测状态，库里的行留给对方按超时清理
		if len(channelRpmSharedPeers) > 0 {
			channelRpmSharedPeers = make(map[string]*channelRpmPeer)
		}
		channelRpmSharedLastEmpty = false
		publishChannelRpmRemoteLocked(now)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), channelRpmSharedTimeout)
	defer cancel()

	if err := reportChannelRpmLocked(ctx, now); err != nil {
		logChannelRpmSharedErrorLocked(now, "report", err)
	}

	var rows []ChannelRpmReport
	err := DB.WithContext(ctx).Where("node_id <> ?", channelRpmSharedNodeId).Find(&rows).Error
	if err != nil {
		logChannelRpmSharedErrorLocked(now, "load", err)
	} else {
		stale := applyChannelRpmReportsLocked(rows, now)
		for _, row := range stale {
			// 带 seq 条件：对方恰好恢复上报时不误删
			if err := DB.WithContext(ctx).Where("node_id = ? AND seq = ?", row.NodeId, row.Seq).
				Delete(&ChannelRpmReport{}).Error; err != nil {
				logChannelRpmSharedErrorLocked(now, "cleanup", err)
				break
			}
		}
	}
	// 读库失败也要重算：让长时间没有新数据的对方按超时淡出，退回只算本进程
	publishChannelRpmRemoteLocked(now)
}

// reportChannelRpmLocked 上报本进程快照。本进程无流量时只补报一次空快照（清掉上一份非空数据），
// 之后不再写库——对方看到 seq 不再推进会自然不计入，备用容器因此零写入。
func reportChannelRpmLocked(ctx context.Context, now time.Time) error {
	snapshot := getLocalChannelRpmSnapshotAt(now)
	if len(snapshot) == 0 && channelRpmSharedLastEmpty {
		return nil
	}
	payload, err := common.Marshal(snapshot)
	if err != nil {
		return err
	}
	channelRpmSharedSeq++
	report := ChannelRpmReport{
		NodeId:     channelRpmSharedNodeId,
		Seq:        channelRpmSharedSeq,
		Payload:    string(payload),
		ReportedAt: now.Unix(),
	}
	err = DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"seq", "payload", "reported_at"}),
	}).Create(&report).Error
	if err != nil {
		return err
	}
	channelRpmSharedLastEmpty = len(snapshot) == 0
	return nil
}

// applyChannelRpmReportsLocked 用读回的行更新对各进程的观测状态，返回可以从库里删掉的陈旧行。
func applyChannelRpmReportsLocked(rows []ChannelRpmReport, now time.Time) (stale []ChannelRpmReport) {
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		seen[row.NodeId] = true
		peer, ok := channelRpmSharedPeers[row.NodeId]
		if !ok {
			// 首次见到：先记下 seq，等它推进一次再计入
			channelRpmSharedPeers[row.NodeId] = &channelRpmPeer{seq: row.Seq, changedAt: now}
			continue
		}
		if row.Seq != peer.seq {
			var rpm map[string]map[int]int64
			if err := common.UnmarshalJsonStr(row.Payload, &rpm); err != nil {
				// 载荷损坏：当作没有新上报，按超时淡出
				continue
			}
			peer.seq = row.Seq
			peer.changedAt = now
			peer.advanced = true
			peer.rpm = rpm
			continue
		}
		if now.Sub(peer.changedAt) > channelRpmSharedRowRetention {
			stale = append(stale, row)
			delete(channelRpmSharedPeers, row.NodeId)
		}
	}
	// 行已被别的进程清掉：同步丢弃观测状态
	for nodeId := range channelRpmSharedPeers {
		if !seen[nodeId] {
			delete(channelRpmSharedPeers, nodeId)
		}
	}
	return stale
}

// publishChannelRpmRemoteLocked 汇总仍在上报的进程，整体替换远端快照
func publishChannelRpmRemoteLocked(now time.Time) {
	var merged map[string]map[int]int64
	peers := 0
	for _, peer := range channelRpmSharedPeers {
		if !peer.advanced || now.Sub(peer.changedAt) > channelRpmSharedPeerTTL {
			continue
		}
		peers++
		for group, channels := range peer.rpm {
			for channelId, rpm := range channels {
				if rpm <= 0 {
					continue
				}
				if merged == nil {
					merged = make(map[string]map[int]int64)
				}
				if merged[group] == nil {
					merged[group] = make(map[int]int64)
				}
				merged[group][channelId] += rpm
			}
		}
	}
	channelRpmRemotePeers.Store(int64(peers))
	if merged == nil {
		channelRpmRemote.Store(nil)
		return
	}
	channelRpmRemote.Store(&merged)
}

func logChannelRpmSharedErrorLocked(now time.Time, stage string, err error) {
	if now.Sub(channelRpmSharedLastLogTime) < channelRpmSharedLogInterval {
		return
	}
	channelRpmSharedLastLogTime = now
	common.SysError(fmt.Sprintf("shared channel rpm sync failed (%s), falling back to local counts: %v", stage, err))
}
