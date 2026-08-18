package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// 渠道日用量历史回填：日用量表是新增的，上线后前两天的悬浮卡全是 0。
// 使用日志（logs，type=2）里已经有 channel_id / quota / tokens / created_at，
// 按天重算即可把历史补齐。管理员手动触发，不做自动回填，避免每次重启重跑。
//
// 三库兼容：不使用任何数据库方言的日期函数，改在 Go 层按天循环，
// 每天用 created_at 的 unix 区间 + GROUP BY channel_id 聚合，天数即查询次数。

const (
	channelDailyUsageBackfillDefaultDays = 30
	channelDailyUsageBackfillMaxDays     = 90
)

type ChannelDailyUsageBackfillResult struct {
	Days      int    `json:"days"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Rows      int    `json:"rows"`
}

// BackfillChannelDailyUsage 用使用日志重算最近 days 天的渠道日用量。
// 幂等：每天先删该天已有行再整批插入，重复执行结果一致。
func BackfillChannelDailyUsage(days int) (*ChannelDailyUsageBackfillResult, error) {
	if days <= 0 {
		days = channelDailyUsageBackfillDefaultDays
	}
	if days > channelDailyUsageBackfillMaxDays {
		return nil, fmt.Errorf("回填天数最多 %d 天", channelDailyUsageBackfillMaxDays)
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil, errors.New("日志库为 ClickHouse，暂不支持渠道日用量回填")
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	firstDay := today.AddDate(0, 0, -(days - 1))

	// 窗口内尚未落库的内存增量必须丢弃：这些请求都是先写 logs 再累加内存的，
	// 回填按天重算已经把它们算进去了，留着会在下一次落库时二次累加。
	dropCachedChannelDailyUsage(firstDay.Format(ChannelDailyUsageDateLayout), today.Format(ChannelDailyUsageDateLayout))

	rows := 0
	for i := 0; i < days; i++ {
		dayStart := firstDay.AddDate(0, 0, i)
		date := dayStart.Format(ChannelDailyUsageDateLayout)

		var aggregated []*ChannelDailyUsage
		err := LOG_DB.Model(&Log{}).
			Select("channel_id, SUM(quota) AS quota_used, COUNT(*) AS request_count, SUM(prompt_tokens + completion_tokens) AS token_used").
			Where("type = ? AND channel_id > 0 AND created_at >= ? AND created_at < ?",
				LogTypeConsume, dayStart.Unix(), dayStart.AddDate(0, 0, 1).Unix()).
			Group("channel_id").
			Scan(&aggregated).Error
		if err != nil {
			return nil, fmt.Errorf("聚合 %s 的使用日志失败: %w", date, err)
		}

		if err := DB.Where("date = ?", date).Delete(&ChannelDailyUsage{}).Error; err != nil {
			return nil, fmt.Errorf("清理 %s 的旧日用量失败: %w", date, err)
		}
		if len(aggregated) == 0 {
			continue
		}
		timestamp := common.GetTimestamp()
		for _, usage := range aggregated {
			usage.Date = date
			usage.UpdatedAt = timestamp
		}
		if err := DB.CreateInBatches(aggregated, 100).Error; err != nil {
			return nil, fmt.Errorf("写入 %s 的日用量失败: %w", date, err)
		}
		rows += len(aggregated)
	}

	result := &ChannelDailyUsageBackfillResult{
		Days:      days,
		StartDate: firstDay.Format(ChannelDailyUsageDateLayout),
		EndDate:   today.Format(ChannelDailyUsageDateLayout),
		Rows:      rows,
	}
	common.SysLog(fmt.Sprintf("回填渠道日用量完成：%s ~ %s 共 %d 条", result.StartDate, result.EndDate, result.Rows))
	return result, nil
}

func dropCachedChannelDailyUsage(startDate string, endDate string) {
	cacheChannelDailyUsageLock.Lock()
	defer cacheChannelDailyUsageLock.Unlock()
	for key, entry := range cacheChannelDailyUsage {
		if entry.Date >= startDate && entry.Date <= endDate {
			delete(cacheChannelDailyUsage, key)
		}
	}
}
