package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func groupNames(groups []cascadeGroupItem) []string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Name)
	}
	return names
}

func TestSortCascadeGroups(t *testing.T) {
	t.Run("未配置展示顺序时按组名升序（与旧版一致）", func(t *testing.T) {
		groups := []cascadeGroupItem{
			{Name: "kiro-高缓"},
			{Name: "default"},
			{Name: "kiro-特价"},
		}
		sortCascadeGroups(groups, map[string]int{})
		assert.Equal(t, []string{"default", "kiro-特价", "kiro-高缓"}, groupNames(groups))
	})

	t.Run("按配置顺序排，未入列的按组名升序垫底", func(t *testing.T) {
		groups := []cascadeGroupItem{
			{Name: "default"},
			{Name: "kiro-高缓"},
			{Name: "Q特价"},
			{Name: "vip"},
			{Name: "Q高缓"},
		}
		sortCascadeGroups(groups, map[string]int{"Q高缓": 1, "Q特价": 2, "kiro-高缓": 3})
		assert.Equal(t, []string{"Q高缓", "Q特价", "kiro-高缓", "default", "vip"}, groupNames(groups))
	})

	t.Run("孤儿分组一律沉底，即使排在配置列表里", func(t *testing.T) {
		groups := []cascadeGroupItem{
			{Name: "dead", Orphan: true},
			{Name: "default"},
			{Name: "Q高缓"},
		}
		sortCascadeGroups(groups, map[string]int{"dead": 1, "default": 2, "Q高缓": 3})
		assert.Equal(t, []string{"default", "Q高缓", "dead"}, groupNames(groups))
	})

	t.Run("配置里含已删除分组不影响其余顺序", func(t *testing.T) {
		groups := []cascadeGroupItem{
			{Name: "default"},
			{Name: "Q特价"},
			{Name: "Q高缓"},
		}
		sortCascadeGroups(groups, map[string]int{"已删除组": 1, "Q高缓": 2, "Q特价": 3})
		assert.Equal(t, []string{"Q高缓", "Q特价", "default"}, groupNames(groups))
	})
}

func TestNormalizeGroupSequence(t *testing.T) {
	assert.Equal(t,
		[]string{"Q高缓", "Q特价", "default"},
		normalizeGroupSequence([]string{" Q高缓 ", "", "Q特价", "Q高缓", "  ", "default"}),
	)
	assert.Equal(t, []string{}, normalizeGroupSequence(nil))
}

// noChannelGroups 用于不涉及旧版水位线落地的用例
func noChannelGroups(int) []string { return nil }

func TestMergeCascadeLimitsIsExclusivePerChannel(t *testing.T) {
	current := cascadeLimits{
		LegacyRpm: map[int]int{},
		GroupRpm: map[string]map[int]int{
			"高缓": {34: 20, 49: 8},
			"特价": {34: 10},
		},
		DailyQuota: map[int]int64{39: 50000000},
	}

	next, err := mergeCascadeLimits(current, []cascadeLimitItem{
		{ChannelId: 34, DailyQuota: 100000000}, // 水位线 → 每日额度：清掉全部分组的水位线
		{ChannelId: 39, Group: "高缓", Rpm: 30},  // 每日额度 → 水位线：清掉额度
		{ChannelId: 53, Group: "特价"},           // 未配置渠道清除：无副作用
	}, noChannelGroups)
	assert.NoError(t, err)
	assert.Equal(t, map[string]map[int]int{"高缓": {39: 30, 49: 8}}, next.GroupRpm)
	assert.Equal(t, map[int]int64{34: 100000000}, next.DailyQuota)

	// 入参 map 不被就地修改（它们是线上配置本体）
	assert.Equal(t, map[string]map[int]int{"高缓": {34: 20, 49: 8}, "特价": {34: 10}}, current.GroupRpm)
	assert.Equal(t, map[int]int64{39: 50000000}, current.DailyQuota)
}

func TestMergeCascadeLimitsGroupsAreIndependent(t *testing.T) {
	current := cascadeLimits{
		GroupRpm: map[string]map[int]int{"高缓": {59: 20}, "特价": {59: 20}},
	}

	next, err := mergeCascadeLimits(current, []cascadeLimitItem{
		{ChannelId: 59, Group: "特价", Rpm: 10},
	}, noChannelGroups)
	assert.NoError(t, err)
	assert.Equal(t, map[string]map[int]int{"高缓": {59: 20}, "特价": {59: 10}}, next.GroupRpm)

	// 带分组清除只清本组；分组清空后组名一并删掉
	next, err = mergeCascadeLimits(next, []cascadeLimitItem{
		{ChannelId: 59, Group: "特价"},
	}, noChannelGroups)
	assert.NoError(t, err)
	assert.Equal(t, map[string]map[int]int{"高缓": {59: 20}}, next.GroupRpm)
}

func TestMergeCascadeLimitsMaterializesLegacyRpm(t *testing.T) {
	current := cascadeLimits{
		LegacyRpm:  map[int]int{59: 20, 35: 5},
		GroupRpm:   map[string]map[int]int{},
		DailyQuota: map[int]int64{},
	}
	groups := func(channelId int) []string {
		if channelId == 59 {
			return []string{"高缓", "特价"}
		}
		return []string{"特价"}
	}

	// 首次改动 59：旧值 20 先落到它所在的每个分组，再按请求改特价为 10
	next, err := mergeCascadeLimits(current, []cascadeLimitItem{
		{ChannelId: 59, Group: "特价", Rpm: 10},
	}, groups)
	assert.NoError(t, err)
	assert.Equal(t, map[string]map[int]int{"高缓": {59: 20}, "特价": {59: 10}}, next.GroupRpm)
	assert.Equal(t, map[int]int{35: 5}, next.LegacyRpm, "未改动的渠道保留旧值")

	// 首次就清除：落地后再清本组，其他分组保留旧值，不会回落旧值
	next, err = mergeCascadeLimits(next, []cascadeLimitItem{
		{ChannelId: 35, Group: "特价"},
	}, groups)
	assert.NoError(t, err)
	assert.Empty(t, next.LegacyRpm)
	assert.Equal(t, map[string]map[int]int{"高缓": {59: 20}, "特价": {59: 10}}, next.GroupRpm)
}

func TestMergeCascadeLimitsClearsAllWithoutGroup(t *testing.T) {
	next, err := mergeCascadeLimits(cascadeLimits{
		LegacyRpm:  map[int]int{34: 20},
		GroupRpm:   map[string]map[int]int{"高缓": {34: 20, 49: 8}, "特价": {34: 10}},
		DailyQuota: map[int]int64{34: 1},
	}, []cascadeLimitItem{
		{ChannelId: 34},
	}, func(int) []string { return []string{"高缓", "特价"} })
	assert.NoError(t, err)
	assert.Empty(t, next.LegacyRpm)
	assert.Equal(t, map[string]map[int]int{"高缓": {49: 8}}, next.GroupRpm)
	assert.Empty(t, next.DailyQuota)
}

func TestMergeCascadeLimitsRejectsInvalid(t *testing.T) {
	_, err := mergeCascadeLimits(cascadeLimits{}, []cascadeLimitItem{{ChannelId: 34, Group: "高缓", Rpm: 20, DailyQuota: 1}}, noChannelGroups)
	assert.Error(t, err, "两种限流不能同时设置")

	_, err = mergeCascadeLimits(cascadeLimits{}, []cascadeLimitItem{{ChannelId: 0, Group: "高缓", Rpm: 20}}, noChannelGroups)
	assert.Error(t, err, "非法渠道 ID")

	_, err = mergeCascadeLimits(cascadeLimits{}, []cascadeLimitItem{{ChannelId: 34, Rpm: 20}}, noChannelGroups)
	assert.Error(t, err, "设置水位线须指定分组")
}

func TestCascadeShouldRetryCapacityError(t *testing.T) {
	setting := operation_setting.GetCascadeSetting()
	orig := *setting
	t.Cleanup(func() { *setting = orig })
	setting.ExtraFaultStatusCodes = nil
	setting.ExtraFaultKeywords = nil
	setting.IgnoreFaultKeywords = nil
	setting.CapacityDetourKeywords = []string{"native input budget"}

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{}
	capacityErr := types.NewErrorWithStatusCode(errors.New("require approximately 136875 input tokens, exceeding the native input budget 124000"), types.ErrorCodeBadResponse, http.StatusBadRequest)
	plainErr := types.NewErrorWithStatusCode(errors.New("invalid request body"), types.ErrorCodeBadResponse, http.StatusBadRequest)

	assert.True(t, cascadeShouldRetry(ctx, info, capacityErr, 3), "容量类 400 应换渠道重试")
	assert.False(t, cascadeShouldRetry(ctx, info, capacityErr, 0), "无剩余重试次数")
	assert.False(t, cascadeShouldRetry(ctx, info, plainErr, 3), "普通 400 原样返回")

	setting.CapacityDetourKeywords = nil
	assert.False(t, cascadeShouldRetry(ctx, info, capacityErr, 3), "未配置关键词时行为不变")
}
