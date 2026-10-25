package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 归属模式（home_mode）单测：绕行不改写归属、首次落位记成功渠道、故障不清归属、默认规则用户级键。
// 详见 docs/channel/cascade-failover.md §2.4

func setChannelAffinityHomeModeForTest(t *testing.T, enabled bool, homeMode bool, switchOnSuccess bool) {
	t.Helper()
	setting := operation_setting.GetChannelAffinitySetting()
	require.NotNil(t, setting)
	origEnabled, origHome, origSwitch := setting.Enabled, setting.HomeMode, setting.SwitchOnSuccess
	setting.Enabled = enabled
	setting.HomeMode = homeMode
	setting.SwitchOnSuccess = switchOnSuccess
	t.Cleanup(func() {
		setting.Enabled = origEnabled
		setting.HomeMode = origHome
		setting.SwitchOnSuccess = origSwitch
	})
}

func newHomeAffinityContextForTest(t *testing.T, ruleName string, tag string) (*gin.Context, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cacheKeySuffix := fmt.Sprintf("%s:default:home-%s-%d", ruleName, tag, time.Now().UnixNano())
	cacheKeyFull := channelAffinityCacheNamespace + ":" + cacheKeySuffix
	t.Cleanup(func() {
		_, _ = getChannelAffinityCache().DeleteMany([]string{cacheKeySuffix})
	})
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		CacheKey:       cacheKeyFull,
		TTLSeconds:     60,
		RuleName:       ruleName,
		SkipRetry:      true,
		UsingGroup:     "default",
		ModelName:      "claude-sonnet-5",
		RequestPath:    "/v1/messages",
		KeySourceType:  "context_int",
		KeySourceKey:   "id",
		KeyHint:        "93",
		KeyFingerprint: "deadbeef",
	})
	return ctx, cacheKeySuffix
}

func getAffinityCacheValueForTest(t *testing.T, cacheKeySuffix string) (int, bool) {
	t.Helper()
	v, found, err := getChannelAffinityCache().Get(cacheKeySuffix)
	require.NoError(t, err)
	return v, found
}

func TestRecordChannelAffinity_HomeModeDetourKeepsHome(t *testing.T) {
	setChannelAffinityHomeModeForTest(t, true, true, true)
	ctx, key := newHomeAffinityContextForTest(t, "claude user", "detour")

	// 归属 #34 熔断中 → 绕行到 #53 成功
	MarkChannelAffinityDetour(ctx, 34, "tripped")
	ctx.Set("channel_id", 53)
	require.False(t, ShouldSkipRetryAfterChannelAffinityFailure(ctx), "绕行不启用亲和 skip-retry")

	RecordChannelAffinity(ctx, 53)

	got, found := getAffinityCacheValueForTest(t, key)
	require.True(t, found)
	require.Equal(t, 34, got, "绕行成功后应写回归属渠道而非实际服务渠道")

	info, ok := ctx.Get(ginKeyChannelAffinityLogInfo)
	require.True(t, ok)
	infoMap, ok := info.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, true, infoMap["detour"])
	require.Equal(t, "tripped", infoMap["detour_reason"])
	require.Equal(t, 34, infoMap["home_channel_id"])
}

func TestRecordChannelAffinity_HomeModeInRequestRetryKeepsHome(t *testing.T) {
	setChannelAffinityHomeModeForTest(t, true, true, true)
	ctx, key := newHomeAffinityContextForTest(t, "claude user", "retry")

	// 归属 #34 命中 → 请求内故障重试切到 #49 成功
	MarkChannelAffinityUsed(ctx, "default", 34)
	require.True(t, ShouldSkipRetryAfterChannelAffinityFailure(ctx))
	require.False(t, ClearCurrentChannelAffinityCacheOnFault(ctx), "归属模式下故障不清归属")
	require.False(t, ShouldSkipRetryAfterChannelAffinityFailure(ctx), "故障后应解除 skip-retry 让级联切换")
	ctx.Set("channel_id", 49)

	RecordChannelAffinity(ctx, 34)

	got, found := getAffinityCacheValueForTest(t, key)
	require.True(t, found)
	require.Equal(t, 34, got, "请求内重试成功的渠道不应改写归属")
}

func TestRecordChannelAffinity_HomeModeFirstPlacementUsesSuccessChannel(t *testing.T) {
	setChannelAffinityHomeModeForTest(t, true, true, false)
	ctx, key := newHomeAffinityContextForTest(t, "claude user", "first")

	// 无归属：首选 #34 失败、#49 成功 → 归属应是 #49（即便 switch_on_success 关闭）
	ctx.Set("channel_id", 49)
	RecordChannelAffinity(ctx, 34)

	got, found := getAffinityCacheValueForTest(t, key)
	require.True(t, found)
	require.Equal(t, 49, got)
}

func TestRecordChannelAffinity_LegacyModeUnchanged(t *testing.T) {
	// 旧模式 + switch_on_success：记实际成功渠道
	setChannelAffinityHomeModeForTest(t, true, false, true)
	ctx, key := newHomeAffinityContextForTest(t, "claude user", "legacy-switch")
	MarkChannelAffinityUsed(ctx, "default", 34)
	ctx.Set("channel_id", 49)
	RecordChannelAffinity(ctx, 34)
	got, found := getAffinityCacheValueForTest(t, key)
	require.True(t, found)
	require.Equal(t, 49, got)

	// 旧模式 + switch_on_success 关闭：记传入渠道
	setChannelAffinityHomeModeForTest(t, true, false, false)
	ctx2, key2 := newHomeAffinityContextForTest(t, "claude user", "legacy-noswitch")
	MarkChannelAffinityUsed(ctx2, "default", 34)
	ctx2.Set("channel_id", 49)
	RecordChannelAffinity(ctx2, 34)
	got2, found2 := getAffinityCacheValueForTest(t, key2)
	require.True(t, found2)
	require.Equal(t, 34, got2)
}

func TestClearCurrentChannelAffinityCacheOnFault_LegacyModeClears(t *testing.T) {
	setChannelAffinityHomeModeForTest(t, true, false, true)
	ctx, key := newHomeAffinityContextForTest(t, "claude user", "legacy-clear")
	require.NoError(t, getChannelAffinityCache().SetWithTTL(key, 34, time.Minute))
	MarkChannelAffinityUsed(ctx, "default", 34)

	require.True(t, ClearCurrentChannelAffinityCacheOnFault(ctx))
	_, found := getAffinityCacheValueForTest(t, key)
	require.False(t, found, "旧模式下故障应清除亲和")
}

func TestClearCurrentChannelAffinityCacheOnFault_HomeModeKeeps(t *testing.T) {
	setChannelAffinityHomeModeForTest(t, true, true, true)
	ctx, key := newHomeAffinityContextForTest(t, "claude user", "home-keep")
	require.NoError(t, getChannelAffinityCache().SetWithTTL(key, 34, time.Minute))
	MarkChannelAffinityUsed(ctx, "default", 34)

	require.False(t, ClearCurrentChannelAffinityCacheOnFault(ctx))
	got, found := getAffinityCacheValueForTest(t, key)
	require.True(t, found)
	require.Equal(t, 34, got)
}

func TestChannelAffinityHomeModeEnabled_RequiresBothSwitches(t *testing.T) {
	setChannelAffinityHomeModeForTest(t, false, true, true)
	require.False(t, ChannelAffinityHomeModeEnabled(), "亲和总开关关闭时归属模式不生效")
	setChannelAffinityHomeModeForTest(t, true, false, true)
	require.False(t, ChannelAffinityHomeModeEnabled())
	setChannelAffinityHomeModeForTest(t, true, true, true)
	require.True(t, ChannelAffinityHomeModeEnabled())
}

// 首次落位判定：命中规则但无归属 → true；命中归属 / 绕行 → false；无规则 meta / 非归属模式 → false
func TestIsChannelAffinityFirstPlacement(t *testing.T) {
	setChannelAffinityHomeModeForTest(t, true, true, true)

	ctx, _ := newHomeAffinityContextForTest(t, "claude user", "placement")
	require.True(t, IsChannelAffinityFirstPlacement(ctx), "命中规则但缓存 miss = 首次落位")

	usedCtx, _ := newHomeAffinityContextForTest(t, "claude user", "placement-used")
	MarkChannelAffinityUsed(usedCtx, "default", 34)
	require.False(t, IsChannelAffinityFirstPlacement(usedCtx), "已命中归属不是首次落位")

	detourCtx, _ := newHomeAffinityContextForTest(t, "claude user", "placement-detour")
	MarkChannelAffinityDetour(detourCtx, 34, "tripped")
	require.False(t, IsChannelAffinityFirstPlacement(detourCtx), "绕行流量保留归属，不按首次落位选路")

	gin.SetMode(gin.TestMode)
	noRuleCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.False(t, IsChannelAffinityFirstPlacement(noRuleCtx), "未命中任何亲和规则")
	require.False(t, IsChannelAffinityFirstPlacement(nil))

	setChannelAffinityHomeModeForTest(t, true, false, true)
	require.False(t, IsChannelAffinityFirstPlacement(ctx), "旧模式下不存在首次落位语义")
}

func TestGetChannelAffinityHomeHardWatermarkRatio(t *testing.T) {
	setting := operation_setting.GetChannelAffinitySetting()
	orig := setting.HomeHardWatermarkRatio
	t.Cleanup(func() { setting.HomeHardWatermarkRatio = orig })

	setting.HomeHardWatermarkRatio = 0
	require.Equal(t, 0.0, GetChannelAffinityHomeHardWatermarkRatio())
	setting.HomeHardWatermarkRatio = -1
	require.Equal(t, 0.0, GetChannelAffinityHomeHardWatermarkRatio(), "负数视为不设硬顶")
	setting.HomeHardWatermarkRatio = 2
	require.Equal(t, 2.0, GetChannelAffinityHomeHardWatermarkRatio())
}

// 默认 Claude 规则：用户级键（context_int:id）优先于 metadata.user_id，
// /v1/messages 与 /v1/chat/completions 各有一条规则。
func TestDefaultClaudeRulesUseUserLevelKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setChannelAffinityHomeModeForTest(t, true, false, true)

	cases := []struct {
		path         string
		wantRule     string
		wantTemplate bool
	}{
		{path: "/v1/messages", wantRule: "claude user", wantTemplate: true},
		{path: "/v1/chat/completions", wantRule: "claude user chat", wantTemplate: false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			body := `{"model":"claude-sonnet-5","metadata":{"user_id":"user_abc_session_xyz"},"messages":[]}`
			ctx.Request = httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Set("id", 93)

			_, found := GetPreferredChannelByAffinity(ctx, "claude-sonnet-5", "kiro")
			require.False(t, found, "缓存里没有归属时应返回未命中")

			meta, ok := getChannelAffinityMeta(ctx)
			require.True(t, ok, "规则应匹配并写入亲和上下文")
			require.Equal(t, tc.wantRule, meta.RuleName)
			require.Equal(t, "context_int", meta.KeySourceType)
			require.Equal(t, "id", meta.KeySourceKey)
			require.Equal(t, channelAffinityCacheNamespace+":"+tc.wantRule+":kiro:93", meta.CacheKey,
				"键应为 规则名:分组:用户ID，同一用户同一分组共用一个归属")
			require.Equal(t, tc.wantTemplate, len(meta.ParamTemplate) > 0)
		})
	}
}

// 无用户 ID（如未鉴权上下文）时回落到 metadata.user_id
func TestDefaultClaudeRuleFallsBackToMetadataUserId(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setChannelAffinityHomeModeForTest(t, true, false, true)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	body := `{"model":"claude-sonnet-5","metadata":{"user_id":"user_abc_session_xyz"}}`
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	_, _ = GetPreferredChannelByAffinity(ctx, "claude-sonnet-5", "kiro")
	meta, ok := getChannelAffinityMeta(ctx)
	require.True(t, ok)
	require.Equal(t, "gjson", meta.KeySourceType)
	require.Equal(t, "metadata.user_id", meta.KeySourcePath)
	require.True(t, strings.HasSuffix(meta.CacheKey, ":kiro:user_abc_session_xyz"))
}
