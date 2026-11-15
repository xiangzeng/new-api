package model

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 渠道指标全站口径：各进程（香港 / 美国节点、蓝绿双容器）把自己的分钟桶增量累加到共用数据库的
// 同一行上，编排页读聚合结果，于是错误率 / 首字 / 缓存命中率都是全站合计，进程重启也不丢。
//
// 设计要点（稳定性优先，与共享 RPM 同思路）：
//   - 请求路径零新增 IO：记账仍只改内存，多累加一份「待上报增量」（同一把锁内）。
//   - 后台每 channelMetricsSharedFlushInterval 把增量以「计数 = 计数 + 增量」批量写库；
//     写失败把增量并回去下次重试，超过 24h 的增量直接丢弃，内存不会无限涨。
//   - 读取走一次聚合查询并在进程内缓存 channelMetricsSharedCacheTTL，再叠加本进程尚未上报的增量；
//     数据库不可用时退回只看本进程内存（即改造前的行为）。
//   - 表里不分节点：一行 = 一个渠道的一分钟，只保留 24h 多一点，过期行由主节点定时清理。
//
// 详见 docs/channel/cascade-failover.md

const (
	// channelMetricsSharedFlushInterval 增量写库的间隔
	channelMetricsSharedFlushInterval = 30 * time.Second
	// channelMetricsSharedTimeout 单次写库 / 读库的数据库超时
	channelMetricsSharedTimeout = 5 * time.Second
	// channelMetricsSharedCacheTTL 聚合查询结果的进程内缓存时长
	channelMetricsSharedCacheTTL = 10 * time.Second
	// channelMetricsSharedRetention 分钟桶在库里的保留时长（窗口 24h，多留 1h 余量）
	channelMetricsSharedRetention = 25 * time.Hour
	// channelMetricsSharedCleanupInterval 过期行清理间隔（仅主节点）
	channelMetricsSharedCleanupInterval = time.Hour
	// channelMetricsSharedFutureSkew 读取时容忍其他节点时钟超前的幅度
	channelMetricsSharedFutureSkew = 5 * time.Minute
	// channelMetricsSharedBatchSize 写库时一条多行语句最多带的行数
	channelMetricsSharedBatchSize = 200
	// channelMetricsSharedLogInterval 失败日志的最小间隔，避免数据库故障时刷屏
	channelMetricsSharedLogInterval = time.Minute
)

// ChannelMetricBucket 一个渠道一分钟的全站指标合计
type ChannelMetricBucket struct {
	ChannelId        int   `json:"channel_id" gorm:"primaryKey;autoIncrement:false"`
	MinuteTs         int64 `json:"minute_ts" gorm:"primaryKey;autoIncrement:false;index:idx_channel_metric_buckets_minute_ts"`
	Attempts         int64 `json:"attempts"`
	Faults           int64 `json:"faults"`
	Trips            int64 `json:"trips"`
	Restores         int64 `json:"restores"`
	LatencySumMs     int64 `json:"latency_sum_ms"`
	LatencyCount     int64 `json:"latency_count"`
	TtftSumMs        int64 `json:"ttft_sum_ms"`
	TtftCount        int64 `json:"ttft_count"`
	CacheSamples     int64 `json:"cache_samples"`
	CacheInputTokens int64 `json:"cache_input_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
}

const channelMetricBucketsTable = "channel_metric_buckets"

func (ChannelMetricBucket) TableName() string {
	return channelMetricBucketsTable
}

// channelMetricCounterColumns 计数列，顺序与 channelMinuteBucket.counters() 一致
var channelMetricCounterColumns = []string{
	"attempts", "faults", "trips", "restores",
	"latency_sum_ms", "latency_count", "ttft_sum_ms", "ttft_count",
	"cache_samples", "cache_input_tokens", "cache_read_tokens", "cache_write_tokens",
}

// counters 按 channelMetricCounterColumns 的顺序返回各计数的指针
func (b *channelMinuteBucket) counters() []*int64 {
	return []*int64{
		&b.attempts, &b.faults, &b.trips, &b.restores,
		&b.latencySumMs, &b.latencyCount, &b.ttftSumMs, &b.ttftCount,
		&b.cacheSamples, &b.cacheInputTokens, &b.cacheReadTokens, &b.cacheWriteTokens,
	}
}

type channelMetricsPendingKey struct {
	channelId int
	minuteTs  int64
}

// channelMetricsWindowSums 一个渠道 1h / 24h 两个窗口的原始合计（未做除法）
type channelMetricsWindowSums struct {
	hour channelMinuteBucket
	day  channelMinuteBucket
}

var (
	// channelMetricsPending 本进程尚未写库的增量，受 channelMetricsLock 保护
	channelMetricsPending = make(map[channelMetricsPendingKey]*channelMinuteBucket)

	// 以下状态受 channelMetricsSharedLock 保护
	channelMetricsSharedLock        sync.Mutex
	channelMetricsSharedCache       map[int]*channelMetricsWindowSums
	channelMetricsSharedCacheAt     time.Time
	channelMetricsSharedLastLogTime time.Time
	channelMetricsSharedLastCleanup time.Time
)

// channelMetricsPendingBucketLocked 定位待上报增量里 now 所在分钟的桶。调用方须持 channelMetricsLock 写锁。
func channelMetricsPendingBucketLocked(channelId int, now time.Time) *channelMinuteBucket {
	key := channelMetricsPendingKey{channelId: channelId, minuteTs: now.Unix() / 60 * 60}
	bucket, ok := channelMetricsPending[key]
	if !ok {
		bucket = &channelMinuteBucket{minuteTs: key.minuteTs}
		channelMetricsPending[key] = bucket
	}
	return bucket
}

// StartChannelMetricsSharedSyncLoop 周期性把本进程的指标增量写入共用数据库
func StartChannelMetricsSharedSyncLoop() {
	for {
		time.Sleep(channelMetricsSharedFlushInterval)
		syncChannelMetricsShared(time.Now())
	}
}

func syncChannelMetricsShared(now time.Time) {
	defer func() {
		// 指标只是展示用，任何意外都不能带崩进程
		if r := recover(); r != nil {
			common.SysError(fmt.Sprintf("shared channel metrics sync panic: %v", r))
		}
	}()
	if err := flushChannelMetricsPending(now); err != nil {
		logChannelMetricsSharedError(now, "flush", err)
	}
	cleanupChannelMetricBuckets(now)
}

// FlushChannelMetricsShared 进程退出前把尚未写库的指标增量落库，避免重启丢数
func FlushChannelMetricsShared() {
	now := time.Now()
	if err := flushChannelMetricsPending(now); err != nil {
		common.SysError("failed to flush channel metrics before exit: " + err.Error())
	}
}

// takeChannelMetricsPending 取走全部待上报增量；窗口（24h）之外的直接丢弃
func takeChannelMetricsPending(now time.Time) map[channelMetricsPendingKey]*channelMinuteBucket {
	dayStart := now.Unix() - 24*3600
	channelMetricsLock.Lock()
	defer channelMetricsLock.Unlock()
	if len(channelMetricsPending) == 0 {
		return nil
	}
	taken := make(map[channelMetricsPendingKey]*channelMinuteBucket, len(channelMetricsPending))
	for key, bucket := range channelMetricsPending {
		if key.minuteTs > dayStart && !bucket.isZero() {
			taken[key] = bucket
		}
	}
	channelMetricsPending = make(map[channelMetricsPendingKey]*channelMinuteBucket)
	return taken
}

// restoreChannelMetricsPending 写库失败时把取走的增量并回去，等下次重试
func restoreChannelMetricsPending(taken map[channelMetricsPendingKey]*channelMinuteBucket) {
	channelMetricsLock.Lock()
	defer channelMetricsLock.Unlock()
	for key, bucket := range taken {
		if current, ok := channelMetricsPending[key]; ok {
			current.add(bucket)
		} else {
			channelMetricsPending[key] = bucket
		}
	}
}

func flushChannelMetricsPending(now time.Time) error {
	if DB == nil {
		return nil
	}
	taken := takeChannelMetricsPending(now)
	if len(taken) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), channelMetricsSharedTimeout)
	defer cancel()
	if err := upsertChannelMetricBuckets(DB.WithContext(ctx), taken); err != nil {
		restoreChannelMetricsPending(taken)
		return err
	}

	// 刚写进去的数据要立刻读得到：作废聚合缓存
	channelMetricsSharedLock.Lock()
	channelMetricsSharedCacheAt = time.Time{}
	channelMetricsSharedLock.Unlock()
	return nil
}

// upsertChannelMetricBuckets 把一批分钟桶增量累加到库里对应的行（不存在则插入）。
//
// 整批放进一个事务、每 channelMetricsSharedBatchSize 行一条多行语句：美国节点到数据库单次往返
// 约 150ms，逐行写几十行就会超时。行按主键排序，多个进程同时写同一批行时加锁顺序一致，不会死锁。
// 累加表达式带表名限定：PostgreSQL 在 ON CONFLICT DO UPDATE 里不带限定的列名有歧义。
func upsertChannelMetricBuckets(db *gorm.DB, deltas map[channelMetricsPendingKey]*channelMinuteBucket) error {
	rows := make([]ChannelMetricBucket, 0, len(deltas))
	for key, delta := range deltas {
		rows = append(rows, ChannelMetricBucket{
			ChannelId:        key.channelId,
			MinuteTs:         key.minuteTs,
			Attempts:         delta.attempts,
			Faults:           delta.faults,
			Trips:            delta.trips,
			Restores:         delta.restores,
			LatencySumMs:     delta.latencySumMs,
			LatencyCount:     delta.latencyCount,
			TtftSumMs:        delta.ttftSumMs,
			TtftCount:        delta.ttftCount,
			CacheSamples:     delta.cacheSamples,
			CacheInputTokens: delta.cacheInputTokens,
			CacheReadTokens:  delta.cacheReadTokens,
			CacheWriteTokens: delta.cacheWriteTokens,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ChannelId != rows[j].ChannelId {
			return rows[i].ChannelId < rows[j].ChannelId
		}
		return rows[i].MinuteTs < rows[j].MinuteTs
	})

	// 「本行待插入的值」在 MySQL 里写 VALUES(col)，PostgreSQL / SQLite 里写 excluded.col
	incoming := func(column string) string { return "excluded." + column }
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		incoming = func(column string) string { return "VALUES(" + column + ")" }
	}
	assignments := make(map[string]interface{}, len(channelMetricCounterColumns))
	for _, column := range channelMetricCounterColumns {
		assignments[column] = gorm.Expr(channelMetricBucketsTable + "." + column + " + " + incoming(column))
	}
	onConflict := clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}, {Name: "minute_ts"}},
		DoUpdates: clause.Assignments(assignments),
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(rows); start += channelMetricsSharedBatchSize {
			end := min(start+channelMetricsSharedBatchSize, len(rows))
			batch := rows[start:end]
			if err := tx.Clauses(onConflict).Create(&batch).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// cleanupChannelMetricBuckets 定时删掉窗口之外的分钟桶（仅主节点，避免多节点重复删）
func cleanupChannelMetricBuckets(now time.Time) {
	if DB == nil || !common.IsMasterNode {
		return
	}
	channelMetricsSharedLock.Lock()
	due := now.Sub(channelMetricsSharedLastCleanup) >= channelMetricsSharedCleanupInterval
	if due {
		channelMetricsSharedLastCleanup = now
	}
	channelMetricsSharedLock.Unlock()
	if !due {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), channelMetricsSharedTimeout)
	defer cancel()
	cutoff := now.Add(-channelMetricsSharedRetention).Unix()
	if err := DB.WithContext(ctx).Where("minute_ts < ?", cutoff).Delete(&ChannelMetricBucket{}).Error; err != nil {
		logChannelMetricsSharedError(now, "cleanup", err)
	}
}

// loadChannelMetricsSharedSums 聚合库里各渠道 1h / 24h 两个窗口的合计，结果带进程内缓存
func loadChannelMetricsSharedSums(now time.Time) (map[int]*channelMetricsWindowSums, error) {
	channelMetricsSharedLock.Lock()
	defer channelMetricsSharedLock.Unlock()

	if channelMetricsSharedCache != nil {
		age := now.Sub(channelMetricsSharedCacheAt)
		if age >= 0 && age < channelMetricsSharedCacheTTL {
			return channelMetricsSharedCache, nil
		}
	}

	nowTs := now.Unix()
	hourStart := nowTs - 3600
	dayStart := nowTs - 24*3600
	upper := nowTs + int64(channelMetricsSharedFutureSkew/time.Second)

	// SELECT channel_id, SUM(col)..., SUM(CASE WHEN minute_ts > hourStart THEN col ELSE 0 END)...
	selects := make([]string, 0, 1+2*len(channelMetricCounterColumns))
	args := make([]interface{}, 0, len(channelMetricCounterColumns)+2)
	selects = append(selects, "channel_id")
	for _, column := range channelMetricCounterColumns {
		selects = append(selects, "COALESCE(SUM("+column+"), 0)")
	}
	for _, column := range channelMetricCounterColumns {
		selects = append(selects, "COALESCE(SUM(CASE WHEN minute_ts > ? THEN "+column+" ELSE 0 END), 0)")
		args = append(args, hourStart)
	}
	query := "SELECT " + strings.Join(selects, ", ") + " FROM " + channelMetricBucketsTable +
		" WHERE minute_ts > ? AND minute_ts <= ? GROUP BY channel_id"
	args = append(args, dayStart, upper)

	ctx, cancel := context.WithTimeout(context.Background(), channelMetricsSharedTimeout)
	defer cancel()
	rows, err := DB.WithContext(ctx).Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]*channelMetricsWindowSums)
	for rows.Next() {
		var channelId int
		sums := &channelMetricsWindowSums{}
		dest := make([]interface{}, 0, 1+2*len(channelMetricCounterColumns))
		dest = append(dest, &channelId)
		for _, counter := range sums.day.counters() {
			dest = append(dest, counter)
		}
		for _, counter := range sums.hour.counters() {
			dest = append(dest, counter)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		result[channelId] = sums
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	channelMetricsSharedCache = result
	channelMetricsSharedCacheAt = now
	return result, nil
}

// getSharedChannelMetricsSnapshotAt 全站口径快照：库里的合计 + 本进程尚未写库的增量。
// 第二个返回值为 false 表示数据库不可用，调用方应退回只看本进程内存。
func getSharedChannelMetricsSnapshotAt(now time.Time) (map[int]ChannelMetricsInfo, bool) {
	if DB == nil {
		return nil, false
	}
	stored, err := loadChannelMetricsSharedSums(now)
	if err != nil {
		logChannelMetricsSharedError(now, "load", err)
		return nil, false
	}

	nowTs := now.Unix()
	hourStart := nowTs - 3600
	dayStart := nowTs - 24*3600

	// 缓存里的合计是共享的不可变快照，先拷一份再叠加本进程的待上报增量
	merged := make(map[int]*channelMetricsWindowSums, len(stored))
	for id, sums := range stored {
		copied := *sums
		merged[id] = &copied
	}
	channelMetricsLock.RLock()
	for key, bucket := range channelMetricsPending {
		if key.minuteTs <= dayStart || key.minuteTs > nowTs {
			continue
		}
		sums, ok := merged[key.channelId]
		if !ok {
			sums = &channelMetricsWindowSums{}
			merged[key.channelId] = sums
		}
		sums.day.add(bucket)
		if key.minuteTs > hourStart {
			sums.hour.add(bucket)
		}
	}
	channelMetricsLock.RUnlock()

	snapshot := make(map[int]ChannelMetricsInfo, len(merged))
	for id, sums := range merged {
		if sums.day.isZero() {
			continue
		}
		var info ChannelMetricsInfo
		sums.hour.mergeInto(&info.Hour)
		sums.day.mergeInto(&info.Day)
		finalizeChannelMetricsWindow(&info.Hour, sums.hour.latencyCount, sums.hour.ttftCount)
		finalizeChannelMetricsWindow(&info.Day, sums.day.latencyCount, sums.day.ttftCount)
		snapshot[id] = info
	}
	return snapshot, true
}

func logChannelMetricsSharedError(now time.Time, stage string, err error) {
	channelMetricsSharedLock.Lock()
	defer channelMetricsSharedLock.Unlock()
	if now.Sub(channelMetricsSharedLastLogTime) < channelMetricsSharedLogInterval {
		return
	}
	channelMetricsSharedLastLogTime = now
	common.SysError(fmt.Sprintf("shared channel metrics %s failed, falling back to local metrics: %v", stage, err))
}
