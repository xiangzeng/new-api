package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 定时同步渠道时数据库查询失败，不能把内存里的渠道清空（否则所有模型都报「无可用渠道」）。
func TestInitChannelCacheKeepsCacheWhenDatabaseFails(t *testing.T) {
	previousDB, previousMemoryCache := DB, common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	channel := &Channel{Name: "sync-failure", Key: "k", Status: common.ChannelStatusEnabled, Group: "sync-failure-group", Models: "sync-failure-model"}
	require.NoError(t, DB.Create(channel).Error)
	ability := &Ability{Group: "sync-failure-group", Model: "sync-failure-model", ChannelId: channel.Id, Enabled: true}
	require.NoError(t, DB.Create(ability).Error)
	t.Cleanup(func() {
		DB = previousDB
		DB.Delete(&Channel{}, channel.Id)
		DB.Where("channel_id = ?", channel.Id).Delete(&Ability{})
		InitChannelCache()
		common.MemoryCacheEnabled = previousMemoryCache
	})

	InitChannelCache()
	cached, err := CacheGetChannel(channel.Id)
	require.NoError(t, err)
	require.Equal(t, "sync-failure", cached.Name)

	// 换成一个没有任何表的库，模拟查询报错
	brokenDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = brokenDB
	InitChannelCache()

	cached, err = CacheGetChannel(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, "sync-failure", cached.Name)
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	assert.Contains(t, group2model2channels["sync-failure-group"]["sync-failure-model"], channel.Id)
}
