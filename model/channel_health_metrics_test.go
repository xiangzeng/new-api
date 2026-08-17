package model

import (
	"sync"
	"testing"
	"time"
)

func resetChannelMetricsForTest(t *testing.T) {
	t.Helper()
	channelMetricsLock.Lock()
	channelMetricsRegistry = make(map[int]*channelMetricsRing)
	channelMetricsLock.Unlock()

	t.Cleanup(func() {
		channelMetricsLock.Lock()
		channelMetricsRegistry = make(map[int]*channelMetricsRing)
		channelMetricsLock.Unlock()
	})
}

func TestChannelMetricsWindowAggregation(t *testing.T) {
	resetChannelMetricsForTest(t)
	now := time.Unix(1_700_000_000, 0)

	// 25h 前：应完全落在 24h 窗口外
	recordChannelAttemptAt(1, ChannelAttemptFault, 0, 0, false, now.Add(-25*time.Hour))
	// 2h 前：只进 24h 窗口
	recordChannelAttemptAt(1, ChannelAttemptSuccess, 1000, 200, true, now.Add(-2*time.Hour))
	recordChannelAttemptAt(1, ChannelAttemptFault, 0, 0, false, now.Add(-2*time.Hour))
	recordChannelTransitionAt(1, true, now.Add(-2*time.Hour))
	// 10min 前：进 1h + 24h 窗口
	recordChannelAttemptAt(1, ChannelAttemptSuccess, 3000, 400, true, now.Add(-10*time.Minute))
	recordChannelAttemptAt(1, ChannelAttemptFault, 0, 0, false, now.Add(-10*time.Minute))
	recordChannelAttemptAt(1, ChannelAttemptOther, 0, 0, false, now.Add(-10*time.Minute))
	recordChannelTransitionAt(1, false, now.Add(-10*time.Minute))

	snapshot := getChannelMetricsSnapshotAt(now)
	info, ok := snapshot[1]
	if !ok {
		t.Fatal("channel 1 should be in snapshot")
	}

	if info.Hour.Attempts != 3 || info.Hour.Faults != 1 {
		t.Fatalf("hour window attempts/faults = %d/%d, want 3/1", info.Hour.Attempts, info.Hour.Faults)
	}
	if info.Hour.Trips != 0 || info.Hour.Restores != 1 {
		t.Fatalf("hour window trips/restores = %d/%d, want 0/1", info.Hour.Trips, info.Hour.Restores)
	}
	if info.Hour.AvgLatencyMs != 3000 || info.Hour.AvgTtftMs != 400 {
		t.Fatalf("hour window latency/ttft = %d/%d, want 3000/400", info.Hour.AvgLatencyMs, info.Hour.AvgTtftMs)
	}

	if info.Day.Attempts != 5 || info.Day.Faults != 2 {
		t.Fatalf("day window attempts/faults = %d/%d, want 5/2", info.Day.Attempts, info.Day.Faults)
	}
	if info.Day.Trips != 1 || info.Day.Restores != 1 {
		t.Fatalf("day window trips/restores = %d/%d, want 1/1", info.Day.Trips, info.Day.Restores)
	}
	// 成功两次：(1000+3000)/2、(200+400)/2
	if info.Day.AvgLatencyMs != 2000 || info.Day.AvgTtftMs != 300 {
		t.Fatalf("day window latency/ttft = %d/%d, want 2000/300", info.Day.AvgLatencyMs, info.Day.AvgTtftMs)
	}
	if info.Day.ErrorRate != 0.4 {
		t.Fatalf("day error rate = %v, want 0.4", info.Day.ErrorRate)
	}
}

func TestChannelMetricsOnlySuccessCountsLatency(t *testing.T) {
	resetChannelMetricsForTest(t)
	now := time.Unix(1_700_000_000, 0)

	recordChannelAttemptAt(2, ChannelAttemptFault, 9999, 9999, true, now)
	recordChannelAttemptAt(2, ChannelAttemptOther, 8888, 8888, true, now)
	recordChannelAttemptAt(2, ChannelAttemptSuccess, 500, 0, false, now) // 非流式：无首字

	info := getChannelMetricsSnapshotAt(now)[2]
	if info.Hour.AvgLatencyMs != 500 {
		t.Fatalf("avg latency = %d, want 500 (failures must not count)", info.Hour.AvgLatencyMs)
	}
	if info.Hour.AvgTtftMs != 0 {
		t.Fatalf("avg ttft = %d, want 0 (no ttft samples)", info.Hour.AvgTtftMs)
	}
}

func TestChannelMetricsRingReuseDropsExpiredSlot(t *testing.T) {
	resetChannelMetricsForTest(t)
	base := time.Unix(1_700_000_000, 0)

	// 同一槽位（相隔正好 24h）：新写入应清掉旧数据而不是累加
	recordChannelAttemptAt(3, ChannelAttemptFault, 0, 0, false, base)
	recordChannelAttemptAt(3, ChannelAttemptSuccess, 100, 0, false, base.Add(24*time.Hour))

	info := getChannelMetricsSnapshotAt(base.Add(24 * time.Hour).Add(time.Minute))[3]
	if info.Day.Attempts != 1 || info.Day.Faults != 0 {
		t.Fatalf("day attempts/faults = %d/%d, want 1/0 (expired slot must be reset)", info.Day.Attempts, info.Day.Faults)
	}
}

func TestChannelMetricsStaleRingEvicted(t *testing.T) {
	resetChannelMetricsForTest(t)
	base := time.Unix(1_700_000_000, 0)

	recordChannelAttemptAt(4, ChannelAttemptSuccess, 100, 0, false, base)

	// 25h 后快照：无数据返回，且整环被剔除
	snapshot := getChannelMetricsSnapshotAt(base.Add(25 * time.Hour))
	if _, ok := snapshot[4]; ok {
		t.Fatal("stale channel should not appear in snapshot")
	}
	channelMetricsLock.RLock()
	_, exists := channelMetricsRegistry[4]
	channelMetricsLock.RUnlock()
	if exists {
		t.Fatal("stale ring should be evicted from registry")
	}
}

func TestChannelMetricsInvalidChannelIgnored(t *testing.T) {
	resetChannelMetricsForTest(t)
	now := time.Unix(1_700_000_000, 0)

	recordChannelAttemptAt(0, ChannelAttemptSuccess, 100, 0, false, now)
	recordChannelTransitionAt(-1, true, now)

	if len(getChannelMetricsSnapshotAt(now)) != 0 {
		t.Fatal("invalid channel ids must not create entries")
	}
}

func TestChannelMetricsConcurrentAccess(t *testing.T) {
	resetChannelMetricsForTest(t)

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				RecordChannelAttempt(g%3+1, ChannelAttemptSuccess, 10, 5, true)
				RecordChannelTrip(g%3 + 1)
				if i%50 == 0 {
					GetChannelMetricsSnapshot()
				}
			}
		}(g)
	}
	wg.Wait()

	snapshot := GetChannelMetricsSnapshot()
	var total int64
	for _, info := range snapshot {
		total += info.Day.Attempts
	}
	if total != 8*200 {
		t.Fatalf("total attempts = %d, want %d", total, 8*200)
	}
}
