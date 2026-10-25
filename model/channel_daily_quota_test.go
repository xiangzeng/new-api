package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetChannelTodayQuotaForTest(t *testing.T) {
	t.Helper()
	reset := func() {
		channelTodayQuotaLock.Lock()
		channelTodayQuotaDate = ""
		channelTodayQuota = make(map[int]int64)
		channelTodayQuotaLock.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

// setChannelDailyQuotaCapForTest 设置渠道每日额度上限（测试结束还原）
func setChannelDailyQuotaCapForTest(t *testing.T, caps map[int]int64) {
	t.Helper()
	setting := operation_setting.GetCascadeWatermarkSetting()
	orig := setting.ChannelDailyQuota
	setting.ChannelDailyQuota = caps
	t.Cleanup(func() { setting.ChannelDailyQuota = orig })
}

func TestChannelTodayQuotaAccumulatesAndRollsOver(t *testing.T) {
	resetChannelTodayQuotaForTest(t)

	addChannelTodayQuota(7, "2026-10-04", 100)
	addChannelTodayQuota(7, "2026-10-04", 50)
	assert.Equal(t, int64(150), getChannelTodayQuotaOn(7, "2026-10-04"))
	// 查询日期已到次日、尚无新记账：视为 0（不会把昨天的累计带到今天）
	assert.Equal(t, int64(0), getChannelTodayQuotaOn(7, "2026-10-05"))

	// 次日首笔记账：整表归零重新累计
	addChannelTodayQuota(7, "2026-10-05", 30)
	assert.Equal(t, int64(30), getChannelTodayQuotaOn(7, "2026-10-05"))
	// 跨零点迟到的昨日记账：丢弃，不回拨日期
	addChannelTodayQuota(7, "2026-10-04", 999)
	assert.Equal(t, int64(30), getChannelTodayQuotaOn(7, "2026-10-05"))
}

func TestLogChannelDailyUsageFeedsTodayQuota(t *testing.T) {
	setupChannelDailyUsageTable(t)
	resetChannelTodayQuotaForTest(t)

	LogChannelDailyUsage(29, 25, 40)
	LogChannelDailyUsage(29, 15, 10)
	// 刷库后用量缓存清空，今日累计不受影响
	SaveChannelDailyUsageCache()
	LogChannelDailyUsage(29, 10, 0)

	assert.Equal(t, int64(50), GetChannelTodayQuota(29))
}

func TestSyncChannelTodayQuotaSeedsFromDBAndCache(t *testing.T) {
	setupChannelDailyUsageTable(t)
	resetChannelTodayQuotaForTest(t)

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	require.NoError(t, DB.Create(&ChannelDailyUsage{ChannelId: 39, Date: today, QuotaUsed: 1000, UpdatedAt: common.GetTimestamp()}).Error)
	require.NoError(t, DB.Create(&ChannelDailyUsage{ChannelId: 39, Date: yesterday, QuotaUsed: 5000, UpdatedAt: common.GetTimestamp()}).Error)

	// 启动做种：只认今天的库存 + 未刷库缓存
	SyncChannelTodayQuota()
	assert.Equal(t, int64(1000), GetChannelTodayQuota(39))

	// 本实例的实时记账：同时进缓存与今日累计，对账后不重复计算
	LogChannelDailyUsage(39, 200, 0)
	SyncChannelTodayQuota()
	assert.Equal(t, int64(1200), GetChannelTodayQuota(39))

	// 另一实例（蓝绿切换）退出时刷进库的消耗：对账补上
	require.NoError(t, DB.Model(&ChannelDailyUsage{}).Where("channel_id = ? AND date = ?", 39, today).
		Update("quota_used", 1300).Error)
	SyncChannelTodayQuota()
	assert.Equal(t, int64(1500), GetChannelTodayQuota(39))
}

func TestSyncChannelTodayQuotaNeverLowersLiveTotal(t *testing.T) {
	setupChannelDailyUsageTable(t)
	resetChannelTodayQuotaForTest(t)

	// 内存累计高于库 + 缓存（例如刷库与对账交错时的瞬时少读）：取较大值，不往回调
	addChannelTodayQuota(39, time.Now().Format("2006-01-02"), 800)
	SyncChannelTodayQuota()
	assert.Equal(t, int64(800), GetChannelTodayQuota(39))
}

func TestIsChannelOverDailyQuota(t *testing.T) {
	resetChannelTodayQuotaForTest(t)
	setChannelDailyQuotaCapForTest(t, map[int]int64{1: 100, 2: 0})
	today := time.Now().Format("2006-01-02")

	addChannelTodayQuota(1, today, 99)
	assert.False(t, IsChannelOverDailyQuota(1))
	addChannelTodayQuota(1, today, 1)
	assert.True(t, IsChannelOverDailyQuota(1), "达到上限即视为满额")

	addChannelTodayQuota(2, today, 1_000_000)
	assert.False(t, IsChannelOverDailyQuota(2), "上限为 0 = 不限")
	addChannelTodayQuota(3, today, 1_000_000)
	assert.False(t, IsChannelOverDailyQuota(3), "未配置 = 不限")
}

func TestSelectCascadeChannelSkipsChannelOverDailyQuota(t *testing.T) {
	resetCascadeHealthForTest(t)
	resetChannelRpmRegistryForTest(t)
	resetChannelTodayQuotaForTest(t)
	setCascadeWatermarkForTest(t, true, map[int]int{2: 10})
	setChannelDailyQuotaCapForTest(t, map[int]int64{1: 100})
	today := time.Now().Format("2006-01-02")

	candidates := []*Channel{
		makeCascadeChannel(1, 10), // 每日额度上限
		makeCascadeChannel(2, 5),  // RPM 水位线
		makeCascadeChannel(3, 1),
	}

	addChannelTodayQuota(1, today, 60)
	picked := selectCascadeChannel(testRpmGroup, candidates, map[int]bool{}, 0)
	require.NotNil(t, picked)
	assert.Equal(t, 1, picked.Id, "额度未满")

	// 正常轮：额度满即溢出到下一个渠道
	addChannelTodayQuota(1, today, 40)
	picked = selectCascadeChannel(testRpmGroup, candidates, map[int]bool{}, 0)
	require.NotNil(t, picked)
	assert.Equal(t, 2, picked.Id, "渠道 1 额度满溢出")

	// 打满轮：其余健康渠道全被试过 / 压满时，也不回落到额度已满的渠道
	fillChannelRpmForTest(2, 10)
	picked = selectCascadeChannel(testRpmGroup, candidates, map[int]bool{3: true}, 0)
	require.NotNil(t, picked)
	assert.Equal(t, 2, picked.Id, "打满轮跳过额度已满渠道")

	// 熔断兜底轮：全员熔断时也不选额度已满的渠道（等同禁用）
	MarkChannelHealthFailure(1, "boom")
	MarkChannelHealthFailure(2, "boom")
	MarkChannelHealthFailure(3, "boom")
	picked = selectCascadeChannel(testRpmGroup, candidates, map[int]bool{}, 0)
	require.NotNil(t, picked)
	assert.Equal(t, 2, picked.Id, "兜底轮跳过额度已满渠道")
	// 只剩额度已满的渠道可选：返回 nil，宁可失败也不超支
	assert.Nil(t, selectCascadeChannel(testRpmGroup, candidates, map[int]bool{2: true, 3: true}, 0), "只剩额度已满渠道")
}
