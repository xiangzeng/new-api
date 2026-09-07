package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupChannelUserUsageTable(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Where("1 = 1").Delete(&QuotaData{}).Error)
	CacheQuotaDataLock.Lock()
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()

	t.Cleanup(func() {
		DB.Where("1 = 1").Delete(&QuotaData{})
		CacheQuotaDataLock.Lock()
		CacheQuotaData = make(map[string]*QuotaData)
		CacheQuotaDataLock.Unlock()
	})
}

// 同一用户跨模型/令牌的小时桶要合成一行并按分组拆明细；其他渠道与窗口外的桶不得串入；
// 尚未落库的内存增量要合并；结果按额度降序并受 limit 约束。
func TestGetChannelUserUsageRankingAggregatesByUserAndGroup(t *testing.T) {
	setupChannelUserUsageTable(t)

	seed := []QuotaData{
		{UserID: 1, Username: "alice", ChannelID: 47, ModelName: "opus", UseGroup: "svip", CreatedAt: 3600, Count: 2, Quota: 100, TokenUsed: 40},
		{UserID: 1, Username: "alice", ChannelID: 47, ModelName: "sonnet", UseGroup: "default", CreatedAt: 7200, Count: 3, Quota: 50, TokenUsed: 30},
		{UserID: 1, Username: "alice", ChannelID: 47, ModelName: "haiku", UseGroup: "svip", CreatedAt: 7200, Count: 1, Quota: 20, TokenUsed: 10},
		{UserID: 2, Username: "bob", ChannelID: 47, ModelName: "opus", UseGroup: "svip", CreatedAt: 7200, Count: 1, Quota: 300, TokenUsed: 90},
		{UserID: 3, Username: "carol", ChannelID: 47, ModelName: "opus", UseGroup: "svip", CreatedAt: 3600, Count: 1, Quota: 10, TokenUsed: 5},
		// 其他渠道
		{UserID: 1, Username: "alice", ChannelID: 48, ModelName: "opus", UseGroup: "svip", CreatedAt: 3600, Count: 9, Quota: 999, TokenUsed: 999},
		// 窗口外（下界之前 / 上界之后）
		{UserID: 2, Username: "bob", ChannelID: 47, ModelName: "opus", UseGroup: "svip", CreatedAt: 0, Count: 9, Quota: 999, TokenUsed: 999},
		{UserID: 2, Username: "bob", ChannelID: 47, ModelName: "opus", UseGroup: "svip", CreatedAt: 10800, Count: 9, Quota: 999, TokenUsed: 999},
	}
	for i := range seed {
		require.NoError(t, DB.Create(&seed[i]).Error)
	}
	// 未落库的内存增量：alice 在 svip 再消费 25，dave 是窗口内的新用户，eve 在其他渠道
	LogQuotaData(QuotaDataLogParams{UserID: 1, Username: "alice", ChannelID: 47, ModelName: "opus", UseGroup: "svip", CreatedAt: 7250, Quota: 25, TokenUsed: 5})
	LogQuotaData(QuotaDataLogParams{UserID: 4, Username: "dave", ChannelID: 47, ModelName: "opus", UseGroup: "svip", CreatedAt: 3700, Quota: 60, TokenUsed: 6})
	LogQuotaData(QuotaDataLogParams{UserID: 5, Username: "eve", ChannelID: 48, ModelName: "opus", UseGroup: "svip", CreatedAt: 3700, Quota: 999, TokenUsed: 9})

	rows, err := GetChannelUserUsageRanking(47, 3600, 7200, 0)
	require.NoError(t, err)
	require.Len(t, rows, 4)

	assert.Equal(t, "bob", rows[0].Username)
	assert.Equal(t, int64(300), rows[0].Quota)
	require.Len(t, rows[0].Groups, 1)
	assert.Equal(t, "svip", rows[0].Groups[0].Group)

	alice := rows[1]
	assert.Equal(t, "alice", alice.Username)
	assert.Equal(t, int64(195), alice.Quota, "三个小时桶 + 内存增量")
	assert.Equal(t, int64(7), alice.Count)
	assert.Equal(t, int64(85), alice.TokenUsed)
	require.Len(t, alice.Groups, 2, "按分组拆明细")
	assert.Equal(t, "svip", alice.Groups[0].Group, "分组按消耗降序")
	assert.Equal(t, int64(145), alice.Groups[0].Quota, "同分组跨模型合并，含内存增量")
	assert.Equal(t, int64(4), alice.Groups[0].Count)
	assert.Equal(t, "default", alice.Groups[1].Group)
	assert.Equal(t, int64(50), alice.Groups[1].Quota)

	assert.Equal(t, "dave", rows[2].Username, "只存在于内存增量的用户也要出现")
	assert.Equal(t, int64(60), rows[2].Quota)
	assert.Equal(t, "carol", rows[3].Username)

	limited, err := GetChannelUserUsageRanking(47, 3600, 7200, 2)
	require.NoError(t, err)
	require.Len(t, limited, 2)
	assert.Equal(t, "bob", limited[0].Username)
	assert.Equal(t, "alice", limited[1].Username)

	empty, err := GetChannelUserUsageRanking(99, 3600, 7200, 0)
	require.NoError(t, err)
	assert.Empty(t, empty)
}
