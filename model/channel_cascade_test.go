package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
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

	if got := pickCascadeChannel(candidates, map[int]bool{}, true); got == nil || got.Id != 1 {
		t.Fatalf("pick = %v, want channel 1", got)
	}
	// 首位已试过：取下一个
	if got := pickCascadeChannel(candidates, map[int]bool{1: true}, true); got == nil || got.Id != 2 {
		t.Fatalf("pick = %v, want channel 2", got)
	}
	// 首位熔断：健康轮跳过，兜底轮不跳
	MarkChannelHealthFailure(1, "boom")
	if got := pickCascadeChannel(candidates, map[int]bool{}, true); got == nil || got.Id != 2 {
		t.Fatalf("pick = %v, want channel 2 (tripped skipped)", got)
	}
	if got := pickCascadeChannel(candidates, map[int]bool{}, false); got == nil || got.Id != 1 {
		t.Fatalf("pick = %v, want channel 1 (health ignored in fallback round)", got)
	}
	// 全部试过：返回 nil
	if got := pickCascadeChannel(candidates, map[int]bool{1: true, 2: true, 3: true}, false); got != nil {
		t.Fatalf("pick = %v, want nil", got)
	}
}
