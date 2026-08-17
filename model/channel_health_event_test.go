package model

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func setupChannelHealthEventTable(t *testing.T) {
	t.Helper()
	if err := DB.AutoMigrate(&ChannelHealthEvent{}); err != nil {
		t.Fatalf("failed to migrate channel_health_events: %v", err)
	}
	if err := DB.Where("1 = 1").Delete(&ChannelHealthEvent{}).Error; err != nil {
		t.Fatalf("failed to clean channel_health_events: %v", err)
	}
	lastChannelHealthEventCleanupDay.Store(0)
	t.Cleanup(func() {
		DB.Where("1 = 1").Delete(&ChannelHealthEvent{})
		lastChannelHealthEventCleanupDay.Store(0)
	})
}

func TestChannelHealthEventAppendAndQuery(t *testing.T) {
	setupChannelHealthEventTable(t)
	now := time.Now().Unix()

	// 8d 前：7d 窗口外，不应出现
	appendChannelHealthEventSync(7, ChannelHealthEventTrip, "too old", now-8*86400)
	// 3d 前：进 7d 计数，不进 24h 计数
	appendChannelHealthEventSync(7, ChannelHealthEventTrip, "err A", now-3*86400)
	appendChannelHealthEventSync(7, ChannelHealthEventRestore, "探活连续成功恢复", now-3*86400+60)
	// 1h 前：进 24h + 7d 计数
	appendChannelHealthEventSync(7, ChannelHealthEventTrip, "err B", now-3600)
	appendChannelHealthEventSync(7, ChannelHealthEventManualReset, "管理员手动恢复", now-3000)
	// 其他渠道：不应串入
	appendChannelHealthEventSync(8, ChannelHealthEventTrip, "other channel", now-60)

	result, err := GetChannelHealthEvents(7)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(result.Events) != 4 {
		t.Fatalf("events len = %d, want 4", len(result.Events))
	}
	// 倒序：最近的在前
	if result.Events[0].Event != ChannelHealthEventManualReset || result.Events[3].Event != ChannelHealthEventTrip {
		t.Fatalf("events not ordered desc: first=%s last=%s", result.Events[0].Event, result.Events[3].Event)
	}
	if result.TripCount24h != 1 {
		t.Fatalf("trip_count_24h = %d, want 1", result.TripCount24h)
	}
	if result.TripCount7d != 2 {
		t.Fatalf("trip_count_7d = %d, want 2", result.TripCount7d)
	}
}

func TestChannelHealthEventReasonTruncation(t *testing.T) {
	setupChannelHealthEventTable(t)
	now := time.Now().Unix()

	// 499 字节 ASCII + 一个 3 字节汉字（跨 500 字节边界）：截断后必须仍是合法 UTF-8
	reason := strings.Repeat("a", 499) + "汉"
	appendChannelHealthEventSync(9, ChannelHealthEventTrip, reason, now)

	result, err := GetChannelHealthEvents(9)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(result.Events))
	}
	got := result.Events[0].Reason
	if len(got) > channelHealthEventReasonMaxBytes {
		t.Fatalf("reason len = %d, exceeds max %d", len(got), channelHealthEventReasonMaxBytes)
	}
	if !utf8.ValidString(got) {
		t.Fatal("truncated reason is not valid UTF-8")
	}
}

func TestChannelHealthEventRetentionCleanup(t *testing.T) {
	setupChannelHealthEventTable(t)
	now := time.Now().Unix()

	// 先塞一条超期事件（91d 前），再正常写入触发当日首次清理
	if err := DB.Create(&ChannelHealthEvent{
		ChannelId: 10, Event: ChannelHealthEventTrip, Reason: "expired", CreatedAt: now - 91*86400,
	}).Error; err != nil {
		t.Fatalf("failed to seed expired event: %v", err)
	}
	appendChannelHealthEventSync(10, ChannelHealthEventTrip, "fresh", now)

	var count int64
	if err := DB.Model(&ChannelHealthEvent{}).Where("channel_id = ?", 10).Count(&count).Error; err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 (expired event should be cleaned)", count)
	}

	// 同一天内再次写入不重复清理（仅验证不报错、事件正常入库）
	appendChannelHealthEventSync(10, ChannelHealthEventRestore, "again", now+1)
	if err := DB.Model(&ChannelHealthEvent{}).Where("channel_id = ?", 10).Count(&count).Error; err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func TestChannelHealthEventInvalidChannelIgnored(t *testing.T) {
	setupChannelHealthEventTable(t)

	AppendChannelHealthEvent(0, ChannelHealthEventTrip, "ignored")
	AppendChannelHealthEvent(-1, ChannelHealthEventTrip, "ignored")

	var count int64
	if err := DB.Model(&ChannelHealthEvent{}).Count(&count).Error; err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}
