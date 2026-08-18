package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupChannelDailyUsageTable(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&ChannelDailyUsage{}))
	require.NoError(t, DB.Where("1 = 1").Delete(&ChannelDailyUsage{}).Error)
	require.NoError(t, LOG_DB.Where("1 = 1").Delete(&Log{}).Error)

	cacheChannelDailyUsageLock.Lock()
	cacheChannelDailyUsage = make(map[string]*ChannelDailyUsage)
	cacheChannelDailyUsageLock.Unlock()

	t.Cleanup(func() {
		DB.Where("1 = 1").Delete(&ChannelDailyUsage{})
		LOG_DB.Where("1 = 1").Delete(&Log{})
		cacheChannelDailyUsageLock.Lock()
		cacheChannelDailyUsage = make(map[string]*ChannelDailyUsage)
		cacheChannelDailyUsageLock.Unlock()
	})
}

func startOfToday() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// 悬浮卡读的是「今天」的实时消耗，落库是每 DataExportInterval 分钟一次，
// 因此区间查询必须把尚未落库的内存增量合并进结果，否则今天永远慢一拍。
func TestGetChannelDailyUsageByDateRangeMergesUnflushedCache(t *testing.T) {
	setupChannelDailyUsageTable(t)
	today := startOfToday()
	todayDate := today.Format(ChannelDailyUsageDateLayout)
	yesterdayDate := today.AddDate(0, 0, -1).Format(ChannelDailyUsageDateLayout)

	require.NoError(t, DB.Create(&ChannelDailyUsage{
		ChannelId: 11, Date: yesterdayDate, QuotaUsed: 80, RequestCount: 2, TokenUsed: 40,
	}).Error)
	require.NoError(t, DB.Create(&ChannelDailyUsage{
		ChannelId: 11, Date: todayDate, QuotaUsed: 100, RequestCount: 1, TokenUsed: 30,
	}).Error)

	// 已落库当天的增量应叠加，其他渠道的增量不得串入
	LogChannelDailyUsage(11, 25, 7)
	LogChannelDailyUsage(12, 999, 999)

	usages, err := GetChannelDailyUsageByDateRange(11, yesterdayDate, todayDate)
	require.NoError(t, err)
	require.Len(t, usages, 2)

	assert.Equal(t, todayDate, usages[0].Date, "结果应按日期倒序")
	assert.Equal(t, int64(125), usages[0].QuotaUsed)
	assert.Equal(t, 2, usages[0].RequestCount)
	assert.Equal(t, int64(37), usages[0].TokenUsed)

	assert.Equal(t, yesterdayDate, usages[1].Date)
	assert.Equal(t, int64(80), usages[1].QuotaUsed)

	// 当天还没有落库行时，内存增量本身要能作为一行返回
	usages, err = GetChannelDailyUsageByDateRange(12, yesterdayDate, todayDate)
	require.NoError(t, err)
	require.Len(t, usages, 1)
	assert.Equal(t, todayDate, usages[0].Date)
	assert.Equal(t, int64(999), usages[0].QuotaUsed)
}

// 回填是从 logs 重算日用量，口径必须与实时落库一致（只认消费日志、只认有渠道的请求），
// 且可以重复执行不翻倍——上线补历史时管理员大概率会点不止一次。
func TestBackfillChannelDailyUsageAggregatesLogsIdempotently(t *testing.T) {
	setupChannelDailyUsageTable(t)
	today := startOfToday()
	todayDate := today.Format(ChannelDailyUsageDateLayout)
	yesterday := today.AddDate(0, 0, -1)
	yesterdayDate := yesterday.Format(ChannelDailyUsageDateLayout)

	logs := []*Log{
		{Type: LogTypeConsume, ChannelId: 11, CreatedAt: today.Add(time.Hour).Unix(), Quota: 100, PromptTokens: 10, CompletionTokens: 5},
		{Type: LogTypeConsume, ChannelId: 11, CreatedAt: today.Add(2 * time.Hour).Unix(), Quota: 50, PromptTokens: 1, CompletionTokens: 1},
		{Type: LogTypeConsume, ChannelId: 12, CreatedAt: yesterday.Add(3 * time.Hour).Unix(), Quota: 7, PromptTokens: 2, CompletionTokens: 3},
		// 窗口外：2 天前
		{Type: LogTypeConsume, ChannelId: 11, CreatedAt: today.AddDate(0, 0, -2).Add(time.Hour).Unix(), Quota: 999, PromptTokens: 9, CompletionTokens: 9},
		// 无渠道归属
		{Type: LogTypeConsume, ChannelId: 0, CreatedAt: today.Add(time.Hour).Unix(), Quota: 500, PromptTokens: 5, CompletionTokens: 5},
		// 非消费日志
		{Type: LogTypeError, ChannelId: 11, CreatedAt: today.Add(time.Hour).Unix(), Quota: 400, PromptTokens: 4, CompletionTokens: 4},
	}
	for _, log := range logs {
		require.NoError(t, LOG_DB.Create(log).Error)
	}

	// 窗口内尚未落库的内存增量已经体现在 logs 里，回填后必须被丢弃，否则下次落库会二次累加
	LogChannelDailyUsage(11, 33, 3)

	result, err := BackfillChannelDailyUsage(2)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Days)
	assert.Equal(t, yesterdayDate, result.StartDate)
	assert.Equal(t, todayDate, result.EndDate)
	assert.Equal(t, 2, result.Rows)

	SaveChannelDailyUsageCache()

	assertBackfilled := func(stage string) {
		var usages []*ChannelDailyUsage
		require.NoError(t, DB.Order("date desc").Find(&usages).Error, stage)
		require.Len(t, usages, 2, stage)

		assert.Equal(t, 11, usages[0].ChannelId, stage)
		assert.Equal(t, todayDate, usages[0].Date, stage)
		assert.Equal(t, int64(150), usages[0].QuotaUsed, stage)
		assert.Equal(t, 2, usages[0].RequestCount, stage)
		assert.Equal(t, int64(17), usages[0].TokenUsed, stage)

		assert.Equal(t, 12, usages[1].ChannelId, stage)
		assert.Equal(t, yesterdayDate, usages[1].Date, stage)
		assert.Equal(t, int64(7), usages[1].QuotaUsed, stage)
		assert.Equal(t, 1, usages[1].RequestCount, stage)
		assert.Equal(t, int64(5), usages[1].TokenUsed, stage)
	}
	assertBackfilled("首次回填")

	_, err = BackfillChannelDailyUsage(2)
	require.NoError(t, err)
	assertBackfilled("重复回填")
}

func TestCleanupOldChannelDailyUsageKeepsRetentionWindow(t *testing.T) {
	setupChannelDailyUsageTable(t)
	today := startOfToday()
	staleDate := today.AddDate(0, 0, -(ChannelDailyUsageRetentionDays + 1)).Format(ChannelDailyUsageDateLayout)
	freshDate := today.AddDate(0, 0, -1).Format(ChannelDailyUsageDateLayout)

	require.NoError(t, DB.Create(&ChannelDailyUsage{ChannelId: 11, Date: staleDate, QuotaUsed: 1}).Error)
	require.NoError(t, DB.Create(&ChannelDailyUsage{ChannelId: 11, Date: freshDate, QuotaUsed: 2}).Error)

	CleanupOldChannelDailyUsage(ChannelDailyUsageRetentionDays)

	var usages []*ChannelDailyUsage
	require.NoError(t, DB.Find(&usages).Error)
	require.Len(t, usages, 1)
	assert.Equal(t, freshDate, usages[0].Date)
}
