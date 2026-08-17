package model

import (
	"math"
	"sync"
	"time"
)

// 渠道级分钟桶指标（错误率/熔断次数/耗时/首字），级联编排页可视化用。
// 纯内存环形数组：每渠道 1440 个分钟桶（24h），进程重启清零，与健康注册表同语义。
// 口径与级联熔断判定一致（IsChannelFaultError + ClassifyStreamEnd），探活流量不计入。
// 详见 docs/channel/cascade-failover.md

// ChannelAttemptOutcome 一次 attempt 的归类结果
type ChannelAttemptOutcome int

const (
	// ChannelAttemptSuccess 成功（含耗时/首字采样）
	ChannelAttemptSuccess ChannelAttemptOutcome = iota
	// ChannelAttemptFault 故障类错误 / 安静断流，计入错误率
	ChannelAttemptFault
	// ChannelAttemptOther 非故障类错误（如 400）或无法归因的中断：计 attempt 不计 fault
	ChannelAttemptOther
)

const channelMetricsBucketCount = 24 * 60 // 24h，每分钟一桶

type channelMinuteBucket struct {
	minuteTs     int64 // 桶对应的 unix 分钟起点（秒），槽位复用时校验
	attempts     int64
	faults       int64
	trips        int64
	restores     int64
	latencySumMs int64
	latencyCount int64
	ttftSumMs    int64
	ttftCount    int64
}

type channelMetricsRing struct {
	buckets      [channelMetricsBucketCount]channelMinuteBucket
	lastActiveTs int64 // 最近写入时间（秒），24h 无活动整环剔除
}

var (
	channelMetricsLock     sync.RWMutex
	channelMetricsRegistry = make(map[int]*channelMetricsRing)
)

// ChannelMetricsWindow 单个时间窗口的聚合结果
type ChannelMetricsWindow struct {
	Attempts     int64   `json:"attempts"`
	Faults       int64   `json:"faults"`
	ErrorRate    float64 `json:"error_rate"`
	Trips        int64   `json:"trips"`
	Restores     int64   `json:"restores"`
	AvgLatencyMs int64   `json:"avg_latency_ms"`
	AvgTtftMs    int64   `json:"avg_ttft_ms"`
}

// ChannelMetricsInfo 渠道指标快照（1h/24h 双窗口）
type ChannelMetricsInfo struct {
	Hour ChannelMetricsWindow `json:"1h"`
	Day  ChannelMetricsWindow `json:"24h"`
}

// bucketFor 定位 now 所在分钟桶；槽位属于过期分钟时清零复用。调用方须持写锁。
func (r *channelMetricsRing) bucketFor(now time.Time) *channelMinuteBucket {
	minuteTs := now.Unix() / 60 * 60
	slot := &r.buckets[(minuteTs/60)%channelMetricsBucketCount]
	if slot.minuteTs != minuteTs {
		*slot = channelMinuteBucket{minuteTs: minuteTs}
	}
	r.lastActiveTs = now.Unix()
	return slot
}

func channelMetricsBucket(channelId int, now time.Time) *channelMinuteBucket {
	ring, ok := channelMetricsRegistry[channelId]
	if !ok {
		ring = &channelMetricsRing{}
		channelMetricsRegistry[channelId] = ring
	}
	return ring.bucketFor(now)
}

// RecordChannelAttempt 记录一次真实流量 attempt（探活不要调）。
// 仅成功 attempt 记耗时；首字仅流式且已吐字时有效。
func RecordChannelAttempt(channelId int, outcome ChannelAttemptOutcome, latencyMs int64, ttftMs int64, hasTtft bool) {
	recordChannelAttemptAt(channelId, outcome, latencyMs, ttftMs, hasTtft, time.Now())
}

func recordChannelAttemptAt(channelId int, outcome ChannelAttemptOutcome, latencyMs int64, ttftMs int64, hasTtft bool, now time.Time) {
	if channelId <= 0 {
		return
	}
	channelMetricsLock.Lock()
	defer channelMetricsLock.Unlock()

	bucket := channelMetricsBucket(channelId, now)
	bucket.attempts++
	switch outcome {
	case ChannelAttemptFault:
		bucket.faults++
	case ChannelAttemptSuccess:
		if latencyMs >= 0 {
			bucket.latencySumMs += latencyMs
			bucket.latencyCount++
		}
		if hasTtft && ttftMs >= 0 {
			bucket.ttftSumMs += ttftMs
			bucket.ttftCount++
		}
	}
}

// RecordChannelTrip 记录一次熔断触发（MarkChannelHealthFailure 返回 true 时调用）
func RecordChannelTrip(channelId int) {
	recordChannelTransitionAt(channelId, true, time.Now())
}

// RecordChannelRestore 记录一次恢复（探活/真实流量恢复、手动恢复均算）
func RecordChannelRestore(channelId int) {
	recordChannelTransitionAt(channelId, false, time.Now())
}

func recordChannelTransitionAt(channelId int, trip bool, now time.Time) {
	if channelId <= 0 {
		return
	}
	channelMetricsLock.Lock()
	defer channelMetricsLock.Unlock()

	bucket := channelMetricsBucket(channelId, now)
	if trip {
		bucket.trips++
	} else {
		bucket.restores++
	}
}

func (b *channelMinuteBucket) mergeInto(w *ChannelMetricsWindow) {
	w.Attempts += b.attempts
	w.Faults += b.faults
	w.Trips += b.trips
	w.Restores += b.restores
	w.AvgLatencyMs += b.latencySumMs // 聚合期间先存 sum，finalize 时除以 count
	w.AvgTtftMs += b.ttftSumMs
}

func finalizeChannelMetricsWindow(w *ChannelMetricsWindow, latencyCount, ttftCount int64) {
	if w.Attempts > 0 {
		w.ErrorRate = math.Round(float64(w.Faults)/float64(w.Attempts)*10000) / 10000
	}
	if latencyCount > 0 {
		w.AvgLatencyMs /= latencyCount
	} else {
		w.AvgLatencyMs = 0
	}
	if ttftCount > 0 {
		w.AvgTtftMs /= ttftCount
	} else {
		w.AvgTtftMs = 0
	}
}

// GetChannelMetricsSnapshot 返回全部有数据渠道的 1h/24h 聚合快照，并惰性剔除 24h 无活动的环
func GetChannelMetricsSnapshot() map[int]ChannelMetricsInfo {
	return getChannelMetricsSnapshotAt(time.Now())
}

func getChannelMetricsSnapshotAt(now time.Time) map[int]ChannelMetricsInfo {
	nowTs := now.Unix()
	hourStart := nowTs - 3600
	dayStart := nowTs - 24*3600

	var stale []int
	channelMetricsLock.RLock()
	snapshot := make(map[int]ChannelMetricsInfo, len(channelMetricsRegistry))
	for id, ring := range channelMetricsRegistry {
		if ring.lastActiveTs <= dayStart {
			stale = append(stale, id)
			continue
		}
		var info ChannelMetricsInfo
		var hourLatencyCount, hourTtftCount, dayLatencyCount, dayTtftCount int64
		for i := range ring.buckets {
			bucket := &ring.buckets[i]
			if bucket.minuteTs <= dayStart || bucket.minuteTs > nowTs {
				continue
			}
			bucket.mergeInto(&info.Day)
			dayLatencyCount += bucket.latencyCount
			dayTtftCount += bucket.ttftCount
			if bucket.minuteTs > hourStart {
				bucket.mergeInto(&info.Hour)
				hourLatencyCount += bucket.latencyCount
				hourTtftCount += bucket.ttftCount
			}
		}
		if info.Day.Attempts == 0 && info.Day.Trips == 0 && info.Day.Restores == 0 {
			continue
		}
		finalizeChannelMetricsWindow(&info.Hour, hourLatencyCount, hourTtftCount)
		finalizeChannelMetricsWindow(&info.Day, dayLatencyCount, dayTtftCount)
		snapshot[id] = info
	}
	channelMetricsLock.RUnlock()

	if len(stale) > 0 {
		channelMetricsLock.Lock()
		for _, id := range stale {
			if ring, ok := channelMetricsRegistry[id]; ok && ring.lastActiveTs <= dayStart {
				delete(channelMetricsRegistry, id)
			}
		}
		channelMetricsLock.Unlock()
	}
	return snapshot
}
