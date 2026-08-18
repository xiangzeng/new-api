package model

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// 渠道实时 RPM（60 秒滚动窗口）：级联「压满即溢出」的水位线判定与编排页展示共用。
// 纯内存环形数组：每渠道 60 个 1 秒桶，进程重启清零，与健康注册表同语义。
// 记账口径为「选中即记账」——service 层选出渠道、亲和命中时各 +1，等于实际发往上游的
// attempt 数（一次请求溢出 N 个渠道，则这 N 个渠道各 +1）；探活与渠道测试不走选择器，
// 天然不计入。与仪表盘 RPM（model/log.go 近 60s 消费日志数）不是同一口径。
// 详见 docs/channel/cascade-failover.md

// channelRpmWindowSeconds 滚动窗口长度（秒），同时是环形数组的槽位数
const channelRpmWindowSeconds = 60

// channelRpmIdleSeconds 无流量超过该秒数的渠道整环剔除（快照时惰性回收）
const channelRpmIdleSeconds = 300

type channelRpmBucket struct {
	ts    int64 // 桶对应的 unix 秒，槽位复用时校验
	count int64
}

type channelRpmRing struct {
	buckets      [channelRpmWindowSeconds]channelRpmBucket
	lastActiveTs int64 // 最近记账时间（秒），长期无流量整环剔除
}

var (
	channelRpmLock     sync.RWMutex
	channelRpmRegistry = make(map[int]*channelRpmRing)
)

// sum 汇总 (now-60, now] 窗口内的请求数。过期槽位靠 ts 校验天然排除，无需就地清理，
// 因此读路径是纯读，只持读锁即可。
func (r *channelRpmRing) sum(now int64) int64 {
	cutoff := now - channelRpmWindowSeconds
	var total int64
	for i := range r.buckets {
		bucket := &r.buckets[i]
		if bucket.ts > cutoff && bucket.ts <= now {
			total += bucket.count
		}
	}
	return total
}

// RecordChannelRequest 记录一次渠道被选中（选中即记账；探活、渠道测试不要调）
func RecordChannelRequest(channelId int) {
	recordChannelRequestAt(channelId, time.Now())
}

func recordChannelRequestAt(channelId int, now time.Time) {
	if channelId <= 0 {
		return
	}
	ts := now.Unix()

	channelRpmLock.Lock()
	defer channelRpmLock.Unlock()

	ring, ok := channelRpmRegistry[channelId]
	if !ok {
		ring = &channelRpmRing{}
		channelRpmRegistry[channelId] = ring
	}
	bucket := &ring.buckets[ts%channelRpmWindowSeconds]
	if bucket.ts != ts {
		// 槽位属于 60 秒前的同余秒，清零复用
		*bucket = channelRpmBucket{ts: ts}
	}
	bucket.count++
	ring.lastActiveTs = ts
}

// GetChannelRpm 返回渠道近 60 秒的请求数
func GetChannelRpm(channelId int) int64 {
	return getChannelRpmAt(channelId, time.Now())
}

func getChannelRpmAt(channelId int, now time.Time) int64 {
	if channelId <= 0 {
		return 0
	}
	channelRpmLock.RLock()
	defer channelRpmLock.RUnlock()

	ring, ok := channelRpmRegistry[channelId]
	if !ok {
		return 0
	}
	return ring.sum(now.Unix())
}

// GetChannelRpmSnapshot 返回全部有流量渠道的近 60 秒 RPM，并惰性剔除长期无流量的环
func GetChannelRpmSnapshot() map[int]int64 {
	return getChannelRpmSnapshotAt(time.Now())
}

func getChannelRpmSnapshotAt(now time.Time) map[int]int64 {
	nowTs := now.Unix()
	idleBefore := nowTs - channelRpmIdleSeconds

	var stale []int
	channelRpmLock.RLock()
	snapshot := make(map[int]int64, len(channelRpmRegistry))
	for id, ring := range channelRpmRegistry {
		if ring.lastActiveTs <= idleBefore {
			stale = append(stale, id)
			continue
		}
		if rpm := ring.sum(nowTs); rpm > 0 {
			snapshot[id] = rpm
		}
	}
	channelRpmLock.RUnlock()

	if len(stale) > 0 {
		channelRpmLock.Lock()
		for _, id := range stale {
			// 二次校验：读锁释放后可能又来了流量
			if ring, ok := channelRpmRegistry[id]; ok && ring.lastActiveTs <= idleBefore {
				delete(channelRpmRegistry, id)
			}
		}
		channelRpmLock.Unlock()
	}
	return snapshot
}

// channelRpmWatermark 返回参与选路的渠道水位线：总开关关闭时恒为 0（= 不限流），
// 此时 RPM 照常统计但不影响级联选择，行为与旧版一致。
func channelRpmWatermark(channelId int) int {
	if !operation_setting.GetCascadeSetting().WatermarkEnabled {
		return 0
	}
	return operation_setting.GetChannelRpmWatermark(channelId)
}

// IsChannelOverWatermark 渠道近 60 秒请求数是否已达 RPM 水位线。
// 水位线未配置 / 为 0 / 总开关关闭时恒为 false。
func IsChannelOverWatermark(channelId int) bool {
	watermark := channelRpmWatermark(channelId)
	if watermark <= 0 {
		return false
	}
	return GetChannelRpm(channelId) >= int64(watermark)
}

// channelRpmLoadRatio 渠道负载率 = 当前 RPM / 水位线，供「全员打满」时挑接盘渠道。
// 不限流的渠道返回 0（它本就不会进入打满轮）。
func channelRpmLoadRatio(channelId int) float64 {
	watermark := channelRpmWatermark(channelId)
	if watermark <= 0 {
		return 0
	}
	return float64(GetChannelRpm(channelId)) / float64(watermark)
}
