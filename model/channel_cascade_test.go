package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setCascadeGroupOrdersForTest(t *testing.T, orders map[string][]int) {
	t.Helper()
	setting := operation_setting.GetCascadeOrderSetting()
	orig := setting.GroupOrders
	setting.GroupOrders = orders
	t.Cleanup(func() {
		setting.GroupOrders = orig
	})
}

func makeCascadeChannel(id int, priority int64) *Channel {
	p := priority
	return &Channel{Id: id, Priority: &p}
}

func idsOf(channels []*Channel) []int {
	ids := make([]int, 0, len(channels))
	for _, channel := range channels {
		ids = append(ids, channel.Id)
	}
	return ids
}

func assertIdOrder(t *testing.T, got []*Channel, want []int) {
	t.Helper()
	gotIds := idsOf(got)
	if len(gotIds) != len(want) {
		t.Fatalf("order = %v, want %v", gotIds, want)
	}
	for i := range want {
		if gotIds[i] != want[i] {
			t.Fatalf("order = %v, want %v", gotIds, want)
		}
	}
}

func TestSortCascadeCandidatesConfiguredOrder(t *testing.T) {
	setCascadeGroupOrdersForTest(t, map[string][]int{
		"g": {3, 1}, // 配置顺序与优先级相反
	})

	candidates := []*Channel{
		makeCascadeChannel(1, 10),
		makeCascadeChannel(2, 5), // 未入列：垫底
		makeCascadeChannel(3, 1),
	}
	sortCascadeCandidates("g", candidates)
	assertIdOrder(t, candidates, []int{3, 1, 2})
}

func TestSortCascadeCandidatesFallbackToPriority(t *testing.T) {
	setCascadeGroupOrdersForTest(t, map[string][]int{})

	candidates := []*Channel{
		makeCascadeChannel(5, 1),
		makeCascadeChannel(2, 8),
		makeCascadeChannel(9, 8), // 同优先级：id 升序
		makeCascadeChannel(1, 8),
	}
	sortCascadeCandidates("g", candidates)
	assertIdOrder(t, candidates, []int{1, 2, 9, 5})
}

func TestSortCascadeCandidatesStaleIdsIgnored(t *testing.T) {
	// 列表里有已删渠道（99）和重复 id：不影响其余排序
	setCascadeGroupOrdersForTest(t, map[string][]int{
		"g": {99, 2, 2, 1},
	})

	candidates := []*Channel{
		makeCascadeChannel(1, 10),
		makeCascadeChannel(2, 1),
		makeCascadeChannel(3, 5), // 未入列
	}
	sortCascadeCandidates("g", candidates)
	assertIdOrder(t, candidates, []int{2, 1, 3})
}

func TestSortCascadeCandidatesGroupsIndependent(t *testing.T) {
	// 同一渠道在两个分组排不同位置——本任务的核心诉求
	setCascadeGroupOrdersForTest(t, map[string][]int{
		"special": {51, 49},
		"high":    {49, 51},
	})

	special := []*Channel{makeCascadeChannel(49, 12), makeCascadeChannel(51, 3)}
	high := []*Channel{makeCascadeChannel(49, 12), makeCascadeChannel(51, 3)}
	sortCascadeCandidates("special", special)
	sortCascadeCandidates("high", high)
	assertIdOrder(t, special, []int{51, 49})
	assertIdOrder(t, high, []int{49, 51})
}

func TestPickCascadeChannelStrictOrder(t *testing.T) {
	resetCascadeHealthForTest(t)
	setCascadeGroupOrdersForTest(t, map[string][]int{})

	candidates := []*Channel{
		makeCascadeChannel(1, 10),
		makeCascadeChannel(2, 5),
		makeCascadeChannel(3, 1),
	}

	if got := pickCascadeChannel(testRpmGroup, candidates, map[int]bool{}, true, 0); got == nil || got.Id != 1 {
		t.Fatalf("pick = %v, want channel 1", got)
	}
	// 首位已试过：取下一个
	if got := pickCascadeChannel(testRpmGroup, candidates, map[int]bool{1: true}, true, 0); got == nil || got.Id != 2 {
		t.Fatalf("pick = %v, want channel 2", got)
	}
	// 首位熔断：健康轮跳过，兜底轮不跳
	MarkChannelHealthFailure(1, "boom")
	if got := pickCascadeChannel(testRpmGroup, candidates, map[int]bool{}, true, 0); got == nil || got.Id != 2 {
		t.Fatalf("pick = %v, want channel 2 (tripped skipped)", got)
	}
	if got := pickCascadeChannel(testRpmGroup, candidates, map[int]bool{}, false, 0); got == nil || got.Id != 1 {
		t.Fatalf("pick = %v, want channel 1 (health ignored in fallback round)", got)
	}
	// 全部试过：返回 nil
	if got := pickCascadeChannel(testRpmGroup, candidates, map[int]bool{1: true, 2: true, 3: true}, false, 0); got != nil {
		t.Fatalf("pick = %v, want nil", got)
	}
}

// setCascadeWatermarkForTest 打开水位线总开关并设置旧版渠道级水位线（各分组沿用），
// 分组水位线清空（测试结束还原）
func setCascadeWatermarkForTest(t *testing.T, enabled bool, watermarks map[int]int) {
	t.Helper()
	setGroupCascadeWatermarkForTest(t, enabled, watermarks, map[string]map[int]int{})
}

// setGroupCascadeWatermarkForTest 同时设置旧版渠道级与分组水位线（测试结束还原）
func setGroupCascadeWatermarkForTest(t *testing.T, enabled bool, legacy map[int]int, groups map[string]map[int]int) {
	t.Helper()
	cascade := operation_setting.GetCascadeSetting()
	origEnabled := cascade.WatermarkEnabled
	cascade.WatermarkEnabled = enabled

	watermarkSetting := operation_setting.GetCascadeWatermarkSetting()
	origLegacy := watermarkSetting.ChannelRpm
	origGroups := watermarkSetting.GroupChannelRpm
	watermarkSetting.ChannelRpm = legacy
	watermarkSetting.GroupChannelRpm = groups

	t.Cleanup(func() {
		cascade.WatermarkEnabled = origEnabled
		watermarkSetting.ChannelRpm = origLegacy
		watermarkSetting.GroupChannelRpm = origGroups
	})
}

// fillChannelRpmForTest 把渠道在 testRpmGroup 下的当前 RPM 灌到指定值
func fillChannelRpmForTest(channelId int, count int) {
	fillGroupChannelRpmForTest(testRpmGroup, channelId, count)
}

// fillGroupChannelRpmForTest 把渠道在指定分组下的当前 RPM 灌到指定值
func fillGroupChannelRpmForTest(group string, channelId int, count int) {
	for i := 0; i < count; i++ {
		RecordChannelRequest(group, channelId)
	}
}

func TestSelectCascadeChannelOverflowsOnWatermark(t *testing.T) {
	resetCascadeHealthForTest(t)
	resetChannelRpmRegistryForTest(t)
	setCascadeWatermarkForTest(t, true, map[int]int{1: 10, 2: 10})

	candidates := []*Channel{
		makeCascadeChannel(1, 10),
		makeCascadeChannel(2, 5),
		makeCascadeChannel(3, 1),
	}

	fillChannelRpmForTest(1, 9)
	picked := selectCascadeChannel(testRpmGroup, candidates, map[int]bool{}, 0)
	require.NotNil(t, picked)
	assert.Equal(t, 1, picked.Id, "未达水位线应仍取顺序第一个")

	// 再来一次记账正好压到水位线：自然溢出到下一个渠道
	fillChannelRpmForTest(1, 1)
	picked = selectCascadeChannel(testRpmGroup, candidates, map[int]bool{}, 0)
	require.NotNil(t, picked)
	assert.Equal(t, 2, picked.Id, "渠道 1 压满应溢出到渠道 2")

	// 渠道 2 也压满：溢出到未配置水位线（= 不限流）的渠道 3
	fillChannelRpmForTest(2, 10)
	picked = selectCascadeChannel(testRpmGroup, candidates, map[int]bool{}, 0)
	require.NotNil(t, picked)
	assert.Equal(t, 3, picked.Id, "不限流渠道应兜住溢出")
}

func TestSelectCascadeChannelDisabledWatermarkKeepsLegacyOrder(t *testing.T) {
	resetCascadeHealthForTest(t)
	resetChannelRpmRegistryForTest(t)
	// 总开关关闭：水位线配置存在也不参与选路
	setCascadeWatermarkForTest(t, false, map[int]int{1: 10})

	candidates := []*Channel{
		makeCascadeChannel(1, 10),
		makeCascadeChannel(2, 5),
	}
	fillChannelRpmForTest(1, 100)

	picked := selectCascadeChannel(testRpmGroup, candidates, map[int]bool{}, 0)
	require.NotNil(t, picked)
	assert.Equal(t, 1, picked.Id, "开关关闭时行为应与旧版一致")
}

func TestSelectCascadeChannelAllFullPicksLeastLoaded(t *testing.T) {
	resetCascadeHealthForTest(t)
	resetChannelRpmRegistryForTest(t)
	setCascadeWatermarkForTest(t, true, map[int]int{1: 10, 2: 100, 3: 50})

	candidates := []*Channel{
		makeCascadeChannel(1, 10),
		makeCascadeChannel(2, 5),
		makeCascadeChannel(3, 1),
	}
	// 全员打满，但负载率不同：1 → 2.0，2 → 1.0，3 → 1.2
	fillChannelRpmForTest(1, 20)
	fillChannelRpmForTest(2, 100)
	fillChannelRpmForTest(3, 60)

	picked := selectCascadeChannel(testRpmGroup, candidates, map[int]bool{}, 0)
	require.NotNil(t, picked)
	assert.Equal(t, 2, picked.Id, "负载率最低者应接住溢出")

	// 负载率最低者已试过：交给次低的渠道 3，而不是退回顺序第一个
	picked = selectCascadeChannel(testRpmGroup, candidates, map[int]bool{2: true}, 0)
	require.NotNil(t, picked)
	assert.Equal(t, 3, picked.Id)
}

func TestSelectCascadeChannelAllTrippedIgnoresWatermark(t *testing.T) {
	resetCascadeHealthForTest(t)
	resetChannelRpmRegistryForTest(t)
	setCascadeWatermarkForTest(t, true, map[int]int{1: 10, 2: 10})

	candidates := []*Channel{
		makeCascadeChannel(1, 10),
		makeCascadeChannel(2, 5),
	}
	fillChannelRpmForTest(1, 50)
	fillChannelRpmForTest(2, 50)
	MarkChannelHealthFailure(1, "boom")
	MarkChannelHealthFailure(2, "boom")

	// 全部熔断 + 全部压满：兜底轮忽略两者，按编排顺序取首个
	picked := selectCascadeChannel(testRpmGroup, candidates, map[int]bool{}, 0)
	require.NotNil(t, picked)
	assert.Equal(t, 1, picked.Id, "兜底轮应忽略健康标记与水位线")
}

func TestSelectCascadeChannelPrefersHealthyFullOverTripped(t *testing.T) {
	resetCascadeHealthForTest(t)
	resetChannelRpmRegistryForTest(t)
	setCascadeWatermarkForTest(t, true, map[int]int{1: 10})

	candidates := []*Channel{
		makeCascadeChannel(1, 10), // 健康但压满
		makeCascadeChannel(2, 5),  // 熔断中
	}
	fillChannelRpmForTest(1, 30)
	MarkChannelHealthFailure(2, "boom")

	picked := selectCascadeChannel(testRpmGroup, candidates, map[int]bool{}, 0)
	require.NotNil(t, picked)
	assert.Equal(t, 1, picked.Id, "健康压满应优先于熔断")
}

// 复现生产缺陷（docs/tasks/channel-affinity-home.md §3）：编排 [53,34,49]，53 熔断，
// 34 水位线 20 但当前 25 RPM，49 无水位线。归属模式首次落位若按软水位线会落到 49
// 并被永久归属；按硬顶倍数 2（40 RPM）应落回主渠道 34。
func homePlacementScenarioForTest(t *testing.T, rpm34 int) []*Channel {
	t.Helper()
	resetCascadeHealthForTest(t)
	resetChannelRpmRegistryForTest(t)
	setCascadeWatermarkForTest(t, true, map[int]int{34: 20})

	candidates := []*Channel{
		makeCascadeChannel(53, 30),
		makeCascadeChannel(34, 20),
		makeCascadeChannel(49, 10),
	}
	MarkChannelHealthFailure(53, "boom")
	fillChannelRpmForTest(34, rpm34)
	return candidates
}

func TestSelectCascadeChannelSoftWatermarkOverflowsToBackup(t *testing.T) {
	candidates := homePlacementScenarioForTest(t, 25)
	// ratio 0 = 软水位线：34 已达 20 → 溢出到 49（旧行为，绕行等临时流量仍走这里）
	picked := selectCascadeChannel(testRpmGroup, candidates, map[int]bool{}, 0)
	require.NotNil(t, picked)
	assert.Equal(t, 49, picked.Id, "软水位线溢出")
}

func TestSelectCascadeChannelHardRatioKeepsPrimary(t *testing.T) {
	candidates := homePlacementScenarioForTest(t, 25)
	// ratio 2 = 硬顶 40：34 的 25 RPM 未达硬顶 → 首次落位留在主渠道 34
	picked := selectCascadeChannel(testRpmGroup, candidates, map[int]bool{}, 2)
	require.NotNil(t, picked)
	assert.Equal(t, 34, picked.Id, "硬顶未达，留在主渠道")
	// ratio 1 与软水位线等价
	picked = selectCascadeChannel(testRpmGroup, candidates, map[int]bool{}, 1)
	require.NotNil(t, picked)
	assert.Equal(t, 49, picked.Id, "ratio 1 等价软水位线")
}

func TestSelectCascadeChannelHardRatioStillOverflowsWhenHit(t *testing.T) {
	candidates := homePlacementScenarioForTest(t, 45)
	// 34 已达硬顶 40 → 仍溢出到 49，硬顶不是无限
	picked := selectCascadeChannel(testRpmGroup, candidates, map[int]bool{}, 2)
	require.NotNil(t, picked)
	assert.Equal(t, 49, picked.Id, "达硬顶溢出")
	// 熔断兜底轮忽略 ratio：全部熔断 / 已试过时按顺序取首个未试过的
	MarkChannelHealthFailure(34, "boom")
	MarkChannelHealthFailure(49, "boom")
	picked = selectCascadeChannel(testRpmGroup, candidates, map[int]bool{53: true}, 2)
	require.NotNil(t, picked)
	assert.Equal(t, 34, picked.Id, "兜底轮忽略硬顶")
}

func TestIsChannelOverWatermarkZeroMeansUnlimited(t *testing.T) {
	resetChannelRpmRegistryForTest(t)
	setCascadeWatermarkForTest(t, true, map[int]int{1: 0, 2: 5})

	fillChannelRpmForTest(1, 1000)
	fillChannelRpmForTest(2, 5)

	assert.False(t, IsChannelOverWatermark(testRpmGroup, 1), "水位线为 0 应视为不限流")
	assert.True(t, IsChannelOverWatermark(testRpmGroup, 2), "渠道 2 已达水位线应判定为打满")
}

// 同一渠道挂在两个分组：各组按本组水位线、本组计数独立溢出，互不挤占
func TestSelectCascadeChannelWatermarkIsolatedPerGroup(t *testing.T) {
	resetCascadeHealthForTest(t)
	resetChannelRpmRegistryForTest(t)
	setGroupCascadeWatermarkForTest(t, true, map[int]int{}, map[string]map[int]int{
		"kiro-高缓": {59: 20},
		"kiro-特价": {59: 10},
	})

	candidates := []*Channel{
		makeCascadeChannel(59, 10),
		makeCascadeChannel(53, 5),
	}

	fillGroupChannelRpmForTest("kiro-高缓", 59, 15)
	fillGroupChannelRpmForTest("kiro-特价", 59, 10)
	// 高缓：15 < 20 继续用 59；特价：10 已达 10 溢出到 53。合计 25 不影响任一组
	picked := selectCascadeChannel("kiro-高缓", candidates, map[int]bool{}, 0)
	require.NotNil(t, picked)
	assert.Equal(t, 59, picked.Id, "高缓本组未达水位线")
	picked = selectCascadeChannel("kiro-特价", candidates, map[int]bool{}, 0)
	require.NotNil(t, picked)
	assert.Equal(t, 53, picked.Id, "特价本组已达水位线")

	// 高缓压满到 20：只影响高缓
	fillGroupChannelRpmForTest("kiro-高缓", 59, 5)
	assert.True(t, IsChannelOverWatermark("kiro-高缓", 59), "高缓 59 已达 20，应判定为打满")
	assert.Equal(t, int64(10), GetChannelRpm("kiro-特价", 59), "特价不受高缓记账影响")
}

// 分组未单独设置时沿用旧版渠道级水位线，且对照本组计数；分组设了 0 = 本组不限流
func TestGroupWatermarkFallsBackToLegacy(t *testing.T) {
	resetChannelRpmRegistryForTest(t)
	setGroupCascadeWatermarkForTest(t, true, map[int]int{59: 20}, map[string]map[int]int{
		"kiro-特价": {59: 10},
		"free":    {59: 0},
	})

	fillGroupChannelRpmForTest("kiro-高缓", 59, 19)
	fillGroupChannelRpmForTest("kiro-特价", 59, 10)
	fillGroupChannelRpmForTest("free", 59, 100)

	assert.False(t, IsChannelOverWatermark("kiro-高缓", 59), "高缓沿用旧值 20，19 未达")
	assert.True(t, IsChannelOverWatermark("kiro-特价", 59), "特价单独设了 10，应判定为打满")
	assert.False(t, IsChannelOverWatermark("free", 59), "分组显式设 0 = 不限流，不回落旧值")
}
