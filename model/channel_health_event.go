package model

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
)

// 渠道健康事件（熔断/恢复/手动恢复）落库，级联编排页时间线用。
// 只记状态变迁事件，写入量极小；与内存分钟桶指标（channel_health_metrics.go）互补：
// 指标重启清零，事件跨重启可追溯。详见 docs/channel/cascade-failover.md

const (
	ChannelHealthEventTrip        = "trip"
	ChannelHealthEventRestore     = "restore"
	ChannelHealthEventManualReset = "manual_reset"
)

const (
	channelHealthEventReasonMaxBytes = 500
	channelHealthEventRetentionDays  = 90
	channelHealthEventQueryLimit     = 500
)

type ChannelHealthEvent struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	ChannelId int    `json:"channel_id" gorm:"index:idx_che_channel_time,priority:1"`
	Event     string `json:"event" gorm:"type:varchar(20)"`
	Reason    string `json:"reason" gorm:"type:varchar(512)"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index:idx_che_channel_time,priority:2"`
}

// truncateChannelHealthEventReason 截断 reason 且不切坏 UTF-8（半个多字节字符会被
// MySQL utf8mb4 拒写，导致整条事件丢失）
func truncateChannelHealthEventReason(reason string) string {
	if len(reason) > channelHealthEventReasonMaxBytes {
		reason = reason[:channelHealthEventReasonMaxBytes]
	}
	return strings.ToValidUTF8(reason, "")
}

// lastChannelHealthEventCleanupDay 上次执行过期清理的「天数」（unix 天），跨天首次写入触发清理
var lastChannelHealthEventCleanupDay atomic.Int64

// AppendChannelHealthEvent 异步落一条健康事件（trip/restore/manual_reset）。
// 失败仅记日志，不影响请求路径。
func AppendChannelHealthEvent(channelId int, event string, reason string) {
	if channelId <= 0 {
		return
	}
	gopool.Go(func() {
		appendChannelHealthEventSync(channelId, event, reason, time.Now().Unix())
	})
}

func appendChannelHealthEventSync(channelId int, event string, reason string, nowTs int64) {
	record := &ChannelHealthEvent{
		ChannelId: channelId,
		Event:     event,
		Reason:    truncateChannelHealthEventReason(reason),
		CreatedAt: nowTs,
	}
	if err := DB.Create(record).Error; err != nil {
		common.SysError(fmt.Sprintf("failed to append channel health event (channel #%d, %s): %s", channelId, event, err.Error()))
		return
	}
	cleanupChannelHealthEventsIfNeeded(nowTs)
}

// cleanupChannelHealthEventsIfNeeded 每天首次写入时清理超出保留期的事件
func cleanupChannelHealthEventsIfNeeded(nowTs int64) {
	day := nowTs / 86400
	last := lastChannelHealthEventCleanupDay.Load()
	if day == last || !lastChannelHealthEventCleanupDay.CompareAndSwap(last, day) {
		return
	}
	cutoff := nowTs - channelHealthEventRetentionDays*86400
	if err := DB.Where("created_at < ?", cutoff).Delete(&ChannelHealthEvent{}).Error; err != nil {
		common.SysError("failed to cleanup channel health events: " + err.Error())
	}
}

// ChannelHealthEventsResult 单渠道健康事件查询结果
type ChannelHealthEventsResult struct {
	Events       []ChannelHealthEvent `json:"events"`
	TripCount24h int64                `json:"trip_count_24h"`
	TripCount7d  int64                `json:"trip_count_7d"`
}

// GetChannelHealthEvents 查询指定渠道近 7d 事件（倒序，limit 500）+ 24h/7d 熔断次数
func GetChannelHealthEvents(channelId int) (*ChannelHealthEventsResult, error) {
	now := time.Now().Unix()
	weekStart := now - 7*86400
	dayStart := now - 86400

	result := &ChannelHealthEventsResult{Events: make([]ChannelHealthEvent, 0)}
	err := DB.Where("channel_id = ? AND created_at >= ?", channelId, weekStart).
		Order("created_at DESC").Order("id DESC").
		Limit(channelHealthEventQueryLimit).
		Find(&result.Events).Error
	if err != nil {
		return nil, err
	}
	err = DB.Model(&ChannelHealthEvent{}).
		Where("channel_id = ? AND created_at >= ? AND event = ?", channelId, weekStart, ChannelHealthEventTrip).
		Count(&result.TripCount7d).Error
	if err != nil {
		return nil, err
	}
	err = DB.Model(&ChannelHealthEvent{}).
		Where("channel_id = ? AND created_at >= ? AND event = ?", channelId, dayStart, ChannelHealthEventTrip).
		Count(&result.TripCount24h).Error
	if err != nil {
		return nil, err
	}
	return result, nil
}
