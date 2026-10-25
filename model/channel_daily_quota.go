package model

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// 渠道今日累计消耗（内存）：级联「每日额度上限」的选路判定与编排页展示共用。
// 记账与渠道每日用量同源——LogChannelDailyUsage 在同一把锁内累加，口径即消费日志 quota
// （与渠道「已使用」一致）；但它不随用量缓存刷库清零，选路热路径不碰数据库。
// 日期口径同为 time.Now() 本地日期，跨日自动归零。
// 进程启动与此后每分钟与「库 + 未刷库缓存」取较大值对账：补上启动前已落库的消耗，以及
// 蓝绿切换时另一实例（退出时才刷库）的消耗。对账只往大调，不会把本实例的实时累计调小。
// 锁顺序固定为 cacheChannelDailyUsageLock → channelTodayQuotaLock。
// 详见 docs/channel/cascade-failover.md

// channelTodayQuotaSyncInterval 与「库 + 未刷库缓存」对账的间隔
const channelTodayQuotaSyncInterval = time.Minute

var (
	channelTodayQuotaLock sync.RWMutex
	channelTodayQuotaDate string
	channelTodayQuota     = make(map[int]int64)
)

func channelQuotaDate(now time.Time) string {
	return now.Format("2006-01-02")
}

// rollChannelTodayQuotaLocked 切到 date 对应的日子；date 早于当前日期（跨零点迟到的
// 昨日记账）时返回 false，调用方丢弃。调用方须持有 channelTodayQuotaLock 写锁。
func rollChannelTodayQuotaLocked(date string) bool {
	if channelTodayQuotaDate == date {
		return true
	}
	if date < channelTodayQuotaDate {
		return false
	}
	channelTodayQuotaDate = date
	channelTodayQuota = make(map[int]int64)
	return true
}

// addChannelTodayQuota 累加渠道当日消耗。调用方须持有 cacheChannelDailyUsageLock，
// 保证与对账读到的未刷库缓存是同一快照（否则同一笔消耗可能被算两次）。
func addChannelTodayQuota(channelId int, date string, quota int64) {
	channelTodayQuotaLock.Lock()
	defer channelTodayQuotaLock.Unlock()
	if !rollChannelTodayQuotaLocked(date) {
		return
	}
	channelTodayQuota[channelId] += quota
}

// GetChannelTodayQuota 返回渠道今天累计消耗（quota 单位）
func GetChannelTodayQuota(channelId int) int64 {
	return getChannelTodayQuotaOn(channelId, channelQuotaDate(time.Now()))
}

func getChannelTodayQuotaOn(channelId int, date string) int64 {
	channelTodayQuotaLock.RLock()
	defer channelTodayQuotaLock.RUnlock()
	if channelTodayQuotaDate != date {
		return 0
	}
	return channelTodayQuota[channelId]
}

// GetChannelTodayQuotaSnapshot 返回全部渠道今天的累计消耗，供编排页展示
func GetChannelTodayQuotaSnapshot() map[int]int64 {
	date := channelQuotaDate(time.Now())
	channelTodayQuotaLock.RLock()
	defer channelTodayQuotaLock.RUnlock()
	snapshot := make(map[int]int64, len(channelTodayQuota))
	if channelTodayQuotaDate != date {
		return snapshot
	}
	for id, quota := range channelTodayQuota {
		snapshot[id] = quota
	}
	return snapshot
}

// IsChannelOverDailyQuota 渠道今天累计消耗是否已达每日额度上限。
// 上限未配置 / 为 0 时恒为 false。满额渠道在级联选路中等同禁用，次日自动恢复。
func IsChannelOverDailyQuota(channelId int) bool {
	quotaCap := operation_setting.GetChannelDailyQuotaCap(channelId)
	if quotaCap <= 0 {
		return false
	}
	return GetChannelTodayQuota(channelId) >= quotaCap
}

// SyncChannelTodayQuota 用「库 + 未刷库缓存」对账今日累计（取较大值）。启动时同步调一次做种。
func SyncChannelTodayQuota() {
	syncChannelTodayQuotaOn(channelQuotaDate(time.Now()))
}

func syncChannelTodayQuotaOn(date string) {
	var rows []ChannelDailyUsage
	if err := DB.Select("channel_id", "quota_used").Where("date = ?", date).Find(&rows).Error; err != nil {
		common.SysLog(fmt.Sprintf("failed to sync channel today quota: %v", err))
		return
	}
	// 先读库、再在锁内读缓存：用量只会从缓存流向库，期间若恰好刷库，最多少算
	// （由取较大值兜住），不会重复计算
	totals := make(map[int]int64, len(rows))
	for _, row := range rows {
		totals[row.ChannelId] += row.QuotaUsed
	}

	cacheChannelDailyUsageLock.Lock()
	defer cacheChannelDailyUsageLock.Unlock()
	for _, entry := range cacheChannelDailyUsage {
		if entry.Date == date {
			totals[entry.ChannelId] += entry.QuotaUsed
		}
	}

	channelTodayQuotaLock.Lock()
	defer channelTodayQuotaLock.Unlock()
	if !rollChannelTodayQuotaLocked(date) {
		return
	}
	for id, total := range totals {
		if total > channelTodayQuota[id] {
			channelTodayQuota[id] = total
		}
	}
}

// StartChannelTodayQuotaSyncLoop 周期性对账今日累计（启动时的首次做种由调用方同步完成）
func StartChannelTodayQuotaSyncLoop() {
	for {
		time.Sleep(channelTodayQuotaSyncInterval)
		SyncChannelTodayQuota()
	}
}
