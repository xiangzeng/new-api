package model

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// 渠道日用量：按「渠道 + 自然日（服务器本地时区）」累计额度消耗、请求数与 token 用量，
// 供渠道列表与编排页的「最近 N 天消耗」悬浮卡读取。
// 写入路径与数据看板 quota_data 同一套路：请求侧只累加内存，由 UpdateChannelDailyUsage
// 周期性落库，优雅退出时再 flush 一次，避免高频写库。
// 历史数据可用 BackfillChannelDailyUsage 从使用日志重算（见 channel_daily_usage_backfill.go）。

// ChannelDailyUsageDateLayout 是 Date 字段的唯一口径：服务器本地时区的自然日。
// 落库、查询、回填必须共用它，否则「今天」的边界会对不上。
const ChannelDailyUsageDateLayout = "2006-01-02"

type ChannelDailyUsage struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	ChannelId    int    `json:"channel_id" gorm:"index:idx_cdu_channel_date,priority:1"`
	Date         string `json:"date" gorm:"type:varchar(10);index:idx_cdu_channel_date,priority:2"`
	QuotaUsed    int64  `json:"quota_used" gorm:"bigint;default:0"`
	RequestCount int    `json:"request_count" gorm:"default:0"`
	TokenUsed    int64  `json:"token_used" gorm:"bigint;default:0"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint"`
}

var (
	cacheChannelDailyUsage     = make(map[string]*ChannelDailyUsage)
	cacheChannelDailyUsageLock sync.Mutex
)

// LogChannelDailyUsage 记一笔渠道消耗到内存增量，由消费日志与任务计费日志调用。
func LogChannelDailyUsage(channelId int, quota int, tokenUsed int) {
	if channelId <= 0 {
		return
	}
	date := time.Now().Format(ChannelDailyUsageDateLayout)
	key := fmt.Sprintf("%d-%s", channelId, date)

	cacheChannelDailyUsageLock.Lock()
	defer cacheChannelDailyUsageLock.Unlock()

	if entry, ok := cacheChannelDailyUsage[key]; ok {
		entry.QuotaUsed += int64(quota)
		entry.RequestCount++
		entry.TokenUsed += int64(tokenUsed)
		return
	}
	cacheChannelDailyUsage[key] = &ChannelDailyUsage{
		ChannelId:    channelId,
		Date:         date,
		QuotaUsed:    int64(quota),
		RequestCount: 1,
		TokenUsed:    int64(tokenUsed),
	}
}

// SaveChannelDailyUsageCache 把内存增量累加进数据库，已有行走增量更新，没有则插入。
func SaveChannelDailyUsageCache() {
	cacheChannelDailyUsageLock.Lock()
	defer cacheChannelDailyUsageLock.Unlock()
	size := len(cacheChannelDailyUsage)
	if size == 0 {
		return
	}
	for _, entry := range cacheChannelDailyUsage {
		existing := &ChannelDailyUsage{}
		DB.Where("channel_id = ? AND date = ?", entry.ChannelId, entry.Date).First(existing)
		if existing.Id > 0 {
			err := DB.Model(&ChannelDailyUsage{}).
				Where("channel_id = ? AND date = ?", entry.ChannelId, entry.Date).
				Updates(map[string]interface{}{
					"quota_used":    gorm.Expr("quota_used + ?", entry.QuotaUsed),
					"request_count": gorm.Expr("request_count + ?", entry.RequestCount),
					"token_used":    gorm.Expr("token_used + ?", entry.TokenUsed),
					"updated_at":    common.GetTimestamp(),
				}).Error
			if err != nil {
				common.SysLog(fmt.Sprintf("更新渠道日用量失败: channel=%d date=%s err=%s", entry.ChannelId, entry.Date, err.Error()))
			}
			continue
		}
		entry.UpdatedAt = common.GetTimestamp()
		if err := DB.Create(entry).Error; err != nil {
			common.SysLog(fmt.Sprintf("写入渠道日用量失败: channel=%d date=%s err=%s", entry.ChannelId, entry.Date, err.Error()))
		}
	}
	cacheChannelDailyUsage = make(map[string]*ChannelDailyUsage)
	common.SysLog(fmt.Sprintf("保存渠道日用量数据成功，共保存%d条数据", size))
}

// UpdateChannelDailyUsage 周期性落库循环，由 main 启动。缺了它内存增量重启即丢。
func UpdateChannelDailyUsage() {
	for {
		time.Sleep(time.Duration(common.DataExportInterval) * time.Minute)
		SaveChannelDailyUsageCache()
	}
}

// GetChannelDailyUsageByDateRange 查渠道在 [startDate, endDate] 区间内的分日用量，
// 结果按日期倒序。查询会把尚未落库的内存增量合并进去，保证「今天」实时。
func GetChannelDailyUsageByDateRange(channelId int, startDate string, endDate string) ([]*ChannelDailyUsage, error) {
	var usages []*ChannelDailyUsage
	err := DB.Where("channel_id = ? AND date >= ? AND date <= ?", channelId, startDate, endDate).
		Order("date desc").Find(&usages).Error
	if err != nil {
		return nil, err
	}

	byDate := make(map[string]*ChannelDailyUsage, len(usages))
	for _, usage := range usages {
		byDate[usage.Date] = usage
	}
	cacheChannelDailyUsageLock.Lock()
	for _, entry := range cacheChannelDailyUsage {
		if entry.ChannelId != channelId || entry.Date < startDate || entry.Date > endDate {
			continue
		}
		if usage, ok := byDate[entry.Date]; ok {
			usage.QuotaUsed += entry.QuotaUsed
			usage.RequestCount += entry.RequestCount
			usage.TokenUsed += entry.TokenUsed
			continue
		}
		merged := *entry
		byDate[entry.Date] = &merged
		usages = append(usages, &merged)
	}
	cacheChannelDailyUsageLock.Unlock()

	// 合并内存增量可能追加了新日期，重新按日期倒序排一次
	for i := 1; i < len(usages); i++ {
		for j := i; j > 0 && usages[j-1].Date < usages[j].Date; j-- {
			usages[j-1], usages[j] = usages[j], usages[j-1]
		}
	}
	return usages, nil
}

// CleanupOldChannelDailyUsage 删除保留期之外的日用量数据。
func CleanupOldChannelDailyUsage(retentionDays int) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Format(ChannelDailyUsageDateLayout)
	result := DB.Where("date < ?", cutoff).Delete(&ChannelDailyUsage{})
	if result.Error != nil {
		common.SysLog(fmt.Sprintf("清理渠道日用量数据失败: %s", result.Error.Error()))
		return
	}
	if result.RowsAffected > 0 {
		common.SysLog(fmt.Sprintf("清理渠道日用量数据成功，共清理%d条数据", result.RowsAffected))
	}
}
