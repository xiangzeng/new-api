package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// simulateChannelMetricsProcessRestartForTest 模拟进程重启 / 换到另一个进程：
// 内存环与待上报增量清空，库里的数据保留。
func simulateChannelMetricsProcessRestartForTest() {
	channelMetricsLock.Lock()
	channelMetricsRegistry = make(map[int]*channelMetricsRing)
	channelMetricsPending = make(map[channelMetricsPendingKey]*channelMinuteBucket)
	channelMetricsLock.Unlock()

	channelMetricsSharedLock.Lock()
	channelMetricsSharedCache = nil
	channelMetricsSharedCacheAt = time.Time{}
	channelMetricsSharedLock.Unlock()
}

func TestSharedChannelMetricsIncludesUnflushedLocalData(t *testing.T) {
	resetChannelMetricsForTest(t)
	now := time.Now()

	recordChannelAttemptAt(1, ChannelAttemptSuccess, 1000, 200, true, now)
	recordChannelCacheUsageAt(1, 1000, 900, 50, now)

	// 还没写库：全站快照靠「本进程待上报增量」也能看到
	snapshot, ok := getSharedChannelMetricsSnapshotAt(now)
	require.True(t, ok)
	info := snapshot[1]
	assert.EqualValues(t, 1, info.Hour.Attempts)
	assert.EqualValues(t, 1000, info.Hour.AvgLatencyMs)
	assert.EqualValues(t, 200, info.Hour.AvgTtftMs)
	assert.InDelta(t, 0.9, info.Hour.CacheHitRate, 0.0001)
	assert.InDelta(t, 0.05, info.Hour.CacheWriteRate, 0.0001)
}

func TestSharedChannelMetricsSumsAcrossProcessesAndSurvivesRestart(t *testing.T) {
	resetChannelMetricsForTest(t)
	now := time.Now()

	// 进程 A（比如美国节点）：大量命中缓存的请求
	for i := 0; i < 3; i++ {
		recordChannelAttemptAt(47, ChannelAttemptSuccess, 2000, 400, true, now)
		recordChannelCacheUsageAt(47, 100000, 95000, 4000, now)
	}
	require.NoError(t, flushChannelMetricsPending(now))
	simulateChannelMetricsProcessRestartForTest()

	// 进程 B（比如香港节点）：只经手一个没带缓存的小请求，另有一次故障
	recordChannelAttemptAt(47, ChannelAttemptSuccess, 1000, 100, true, now)
	recordChannelAttemptAt(47, ChannelAttemptFault, 0, 0, false, now)
	recordChannelCacheUsageAt(47, 1000, 0, 0, now)

	// 只看本进程：命中率 0%（改造前编排页看到的就是这个）
	local := getChannelMetricsSnapshotAt(now)[47]
	assert.EqualValues(t, 1, local.Hour.CacheSamples)
	assert.Zero(t, local.Hour.CacheHitRate)

	// 全站口径：两个进程相加
	snapshot, ok := getSharedChannelMetricsSnapshotAt(now)
	require.True(t, ok)
	info := snapshot[47]
	assert.EqualValues(t, 5, info.Hour.Attempts)
	assert.EqualValues(t, 1, info.Hour.Faults)
	assert.InDelta(t, 0.2, info.Hour.ErrorRate, 0.0001)
	assert.EqualValues(t, 4, info.Hour.CacheSamples)
	assert.EqualValues(t, 301000, info.Hour.CacheInputTokens)
	assert.EqualValues(t, 285000, info.Hour.CacheReadTokens)
	assert.InDelta(t, 285000.0/301000.0, info.Hour.CacheHitRate, 0.0001)
	// 耗时 / 首字按样本数加权：(3*2000 + 1000) / 4、(3*400 + 100) / 4
	assert.EqualValues(t, 1750, info.Hour.AvgLatencyMs)
	assert.EqualValues(t, 325, info.Hour.AvgTtftMs)

	// 进程 B 也写库后再「重启」：数据全在库里，不丢
	require.NoError(t, flushChannelMetricsPending(now))
	simulateChannelMetricsProcessRestartForTest()
	snapshot, ok = getSharedChannelMetricsSnapshotAt(now)
	require.True(t, ok)
	assert.EqualValues(t, 5, snapshot[47].Hour.Attempts)
	assert.EqualValues(t, 4, snapshot[47].Hour.CacheSamples)
}

func TestSharedChannelMetricsFlushAccumulatesIntoSameRow(t *testing.T) {
	resetChannelMetricsForTest(t)
	now := time.Now()

	recordChannelAttemptAt(5, ChannelAttemptSuccess, 100, 0, false, now)
	require.NoError(t, flushChannelMetricsPending(now))
	recordChannelAttemptAt(5, ChannelAttemptSuccess, 300, 0, false, now)
	recordChannelTransitionAt(5, true, now)
	require.NoError(t, flushChannelMetricsPending(now))

	var rows []ChannelMetricBucket
	require.NoError(t, DB.Where("channel_id = ?", 5).Find(&rows).Error)
	require.Len(t, rows, 1, "同一渠道同一分钟只有一行，增量累加上去")
	assert.EqualValues(t, 2, rows[0].Attempts)
	assert.EqualValues(t, 400, rows[0].LatencySumMs)
	assert.EqualValues(t, 2, rows[0].LatencyCount)
	assert.EqualValues(t, 1, rows[0].Trips)

	// 没有新增量时不再写库
	require.Nil(t, takeChannelMetricsPending(now))
}

func TestSharedChannelMetricsWindowsAndRetention(t *testing.T) {
	resetChannelMetricsForTest(t)
	now := time.Now()

	recordChannelAttemptAt(9, ChannelAttemptSuccess, 100, 0, false, now.Add(-2*time.Hour))
	recordChannelAttemptAt(9, ChannelAttemptSuccess, 100, 0, false, now.Add(-10*time.Minute))
	// 超出 24h 窗口的增量在取走时直接丢弃，不写库
	recordChannelAttemptAt(9, ChannelAttemptSuccess, 100, 0, false, now.Add(-26*time.Hour))
	require.NoError(t, flushChannelMetricsPending(now))

	var count int64
	require.NoError(t, DB.Model(&ChannelMetricBucket{}).Where("channel_id = ?", 9).Count(&count).Error)
	assert.EqualValues(t, 2, count)

	simulateChannelMetricsProcessRestartForTest()
	snapshot, ok := getSharedChannelMetricsSnapshotAt(now)
	require.True(t, ok)
	assert.EqualValues(t, 1, snapshot[9].Hour.Attempts)
	assert.EqualValues(t, 2, snapshot[9].Day.Attempts)

	// 过期行清理：只在主节点执行
	require.NoError(t, DB.Create(&ChannelMetricBucket{
		ChannelId: 9, MinuteTs: now.Add(-30*time.Hour).Unix() / 60 * 60, Attempts: 7,
	}).Error)
	origMaster := common.IsMasterNode
	t.Cleanup(func() { common.IsMasterNode = origMaster })

	common.IsMasterNode = false
	cleanupChannelMetricBuckets(now)
	require.NoError(t, DB.Model(&ChannelMetricBucket{}).Where("channel_id = ?", 9).Count(&count).Error)
	assert.EqualValues(t, 3, count, "从节点不清理")

	common.IsMasterNode = true
	cleanupChannelMetricBuckets(now)
	require.NoError(t, DB.Model(&ChannelMetricBucket{}).Where("channel_id = ?", 9).Count(&count).Error)
	assert.EqualValues(t, 2, count)
}

func TestSharedChannelMetricsRestoresPendingWhenFlushFails(t *testing.T) {
	resetChannelMetricsForTest(t)
	now := time.Now()

	recordChannelAttemptAt(3, ChannelAttemptSuccess, 100, 0, false, now)
	// 表不存在 → 写库失败，增量必须并回去等下次重试
	require.NoError(t, DB.Migrator().DropTable(&ChannelMetricBucket{}))
	require.Error(t, flushChannelMetricsPending(now))
	recordChannelAttemptAt(3, ChannelAttemptSuccess, 100, 0, false, now)

	// 读库同样失败：对外入口退回只看本进程内存
	_, ok := getSharedChannelMetricsSnapshotAt(now)
	assert.False(t, ok)
	assert.EqualValues(t, 2, GetChannelMetricsSnapshot()[3].Hour.Attempts)

	require.NoError(t, DB.AutoMigrate(&ChannelMetricBucket{}))
	require.NoError(t, flushChannelMetricsPending(now))
	var row ChannelMetricBucket
	require.NoError(t, DB.Where("channel_id = ?", 3).First(&row).Error)
	assert.EqualValues(t, 2, row.Attempts, "失败那次的增量没有丢")
}
