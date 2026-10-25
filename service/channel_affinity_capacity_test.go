package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 容量绕行记忆单测：按「亲和键 + 渠道」记忆、归属渠道超限标绕行且不改归属、过期失效、
// 级联选路排除记忆渠道并在无路可走时放回。详见 docs/channel/cascade-failover.md

// newCapacityRequestContextForTest 模拟同一亲和键的下一次请求（新的 gin 上下文、同一缓存键）
func newCapacityRequestContextForTest(t *testing.T, from *gin.Context) *gin.Context {
	t.Helper()
	meta, ok := getChannelAffinityMeta(from)
	require.True(t, ok)
	return buildChannelAffinityTemplateContextForTest(meta)
}

func cleanupCapacitySkipsForTest(t *testing.T, ctx *gin.Context) {
	t.Helper()
	cacheKey, _, ok := getChannelAffinityContext(ctx)
	require.True(t, ok)
	t.Cleanup(func() {
		channelAffinityCapacitySkipMu.Lock()
		delete(channelAffinityCapacitySkips, cacheKey)
		channelAffinityCapacitySkipMu.Unlock()
	})
}

func TestRecordChannelAffinityCapacityDetour_RemembersPerKeyAndChannel(t *testing.T) {
	setChannelAffinityHomeModeForTest(t, true, true, true)
	ctx, _ := newHomeAffinityContextForTest(t, "claude user", "capacity-remember")
	cleanupCapacitySkipsForTest(t, ctx)

	RecordChannelAffinityCapacityDetour(ctx, 58)

	next := newCapacityRequestContextForTest(t, ctx)
	LoadChannelAffinityCapacitySkips(next)
	require.True(t, IsChannelAffinityCapacitySkipped(next, 58), "同一亲和键的下一次请求应跳过超限渠道")
	require.False(t, IsChannelAffinityCapacitySkipped(next, 53), "其余渠道不受影响")

	other, _ := newHomeAffinityContextForTest(t, "claude user", "capacity-other-user")
	LoadChannelAffinityCapacitySkips(other)
	require.False(t, IsChannelAffinityCapacitySkipped(other, 58), "其他亲和键不受影响")
}

func TestRecordChannelAffinityCapacityDetour_HomeChannelMarksDetourAndKeepsHome(t *testing.T) {
	setChannelAffinityHomeModeForTest(t, true, true, true)
	ctx, key := newHomeAffinityContextForTest(t, "claude user", "capacity-home")
	cleanupCapacitySkipsForTest(t, ctx)

	// 归属 #58 命中 → 超出输入上限 → 请求内换到 #53 成功
	MarkChannelAffinityUsed(ctx, "kiro-高缓", 58)
	RecordChannelAffinityCapacityDetour(ctx, 58)
	ctx.Set("channel_id", 53)
	RecordChannelAffinity(ctx, 58)

	got, found := getAffinityCacheValueForTest(t, key)
	require.True(t, found)
	require.Equal(t, 58, got, "容量绕行不改写归属")

	info, ok := ctx.Get(ginKeyChannelAffinityLogInfo)
	require.True(t, ok)
	infoMap, ok := info.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, true, infoMap["detour"])
	require.Equal(t, "capacity_exceeded", infoMap["detour_reason"])
}

func TestRecordChannelAffinityCapacityDetour_NonHomeChannelNoDetourMark(t *testing.T) {
	setChannelAffinityHomeModeForTest(t, true, true, true)
	ctx, _ := newHomeAffinityContextForTest(t, "claude user", "capacity-nonhome")
	cleanupCapacitySkipsForTest(t, ctx)

	// 归属 #34，请求内重试落到 #58 超限：只记忆，不把本次标成归属绕行
	MarkChannelAffinityUsed(ctx, "kiro-高缓", 34)
	RecordChannelAffinityCapacityDetour(ctx, 58)

	info, _ := ctx.Get(ginKeyChannelAffinityLogInfo)
	infoMap, _ := info.(map[string]interface{})
	require.Nil(t, infoMap["detour_reason"])

	next := newCapacityRequestContextForTest(t, ctx)
	LoadChannelAffinityCapacitySkips(next)
	require.True(t, IsChannelAffinityCapacitySkipped(next, 58))
}

func TestLoadChannelAffinityCapacitySkips_ExpiredIgnoredAndPruned(t *testing.T) {
	setChannelAffinityHomeModeForTest(t, true, true, true)
	ctx, _ := newHomeAffinityContextForTest(t, "claude user", "capacity-expired")
	cleanupCapacitySkipsForTest(t, ctx)
	cacheKey, _, _ := getChannelAffinityContext(ctx)

	RecordChannelAffinityCapacityDetour(ctx, 58)
	channelAffinityCapacitySkipMu.Lock()
	channelAffinityCapacitySkips[cacheKey][58] = time.Now().Add(-time.Second)
	channelAffinityCapacitySkipMu.Unlock()

	next := newCapacityRequestContextForTest(t, ctx)
	LoadChannelAffinityCapacitySkips(next)
	require.False(t, IsChannelAffinityCapacitySkipped(next, 58), "过期后回归属渠道再试")

	// 任意一次记录都会顺带清理过期项
	other, _ := newHomeAffinityContextForTest(t, "claude user", "capacity-expired-other")
	cleanupCapacitySkipsForTest(t, other)
	RecordChannelAffinityCapacityDetour(other, 53)
	channelAffinityCapacitySkipMu.Lock()
	_, stillThere := channelAffinityCapacitySkips[cacheKey]
	channelAffinityCapacitySkipMu.Unlock()
	require.False(t, stillThere)
}

func TestRecordChannelAffinityCapacityDetour_NoAffinityContextNoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	RecordChannelAffinityCapacityDetour(ctx, 58)
	LoadChannelAffinityCapacitySkips(ctx)
	require.False(t, IsChannelAffinityCapacitySkipped(ctx, 58))
	RecordChannelAffinityCapacityDetour(nil, 58)
	require.False(t, IsChannelAffinityCapacitySkipped(nil, 58))
}

func TestCacheGetCascadeChannel_CapacitySkips(t *testing.T) {
	setting := withCascadeSetting(t)
	setting.WatermarkEnabled = false
	require.NoError(t, model.DB.AutoMigrate(&model.Ability{}))
	// 走内存缓存选路（库路径依赖 InitDB 初始化的列名引用）
	origMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = origMemoryCache })

	const group = "capacity-skip-test"
	const modelName = "claude-capacity-test"
	for _, id := range []int{9158, 9153} {
		require.NoError(t, model.DB.Create(&model.Channel{Id: id, Name: "capacity-test", Status: 1, Group: group, Models: modelName}).Error)
		require.NoError(t, model.DB.Create(&model.Ability{Group: group, Model: modelName, ChannelId: id, Enabled: true}).Error)
	}
	t.Cleanup(func() {
		model.DB.Where(map[string]interface{}{"group": group}).Delete(&model.Ability{})
		model.DB.Where("id IN ?", []int{9158, 9153}).Delete(&model.Channel{})
		model.InitChannelCache()
	})
	model.InitChannelCache()
	orderSetting := operation_setting.GetCascadeOrderSetting()
	origOrders := orderSetting.GroupOrders
	orderSetting.GroupOrders = map[string][]int{group: {9158, 9153}}
	t.Cleanup(func() { orderSetting.GroupOrders = origOrders })

	pick := func(tried []string, skips map[int]bool) int {
		t.Helper()
		gin.SetMode(gin.TestMode)
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		if tried != nil {
			ctx.Set("use_channel", tried)
		}
		if skips != nil {
			ctx.Set(ginKeyChannelAffinityCapacitySkips, skips)
		}
		channel, _, err := cacheGetCascadeChannel(&RetryParam{Ctx: ctx, TokenGroup: group, ModelName: modelName}, "")
		require.NoError(t, err)
		if channel == nil {
			return 0
		}
		return channel.Id
	}

	require.Equal(t, 9158, pick(nil, nil), "无记忆按编排顺序")
	require.Equal(t, 9153, pick(nil, map[int]bool{9158: true}), "记忆渠道被跳过")
	require.Equal(t, 9158, pick([]string{"9153"}, map[int]bool{9158: true}), "其余渠道都试过时放回记忆渠道")
	require.Equal(t, 0, pick([]string{"9153", "9158"}, map[int]bool{9158: true}), "全部试过仍无可选")
}
