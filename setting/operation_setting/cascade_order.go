package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// CascadeOrderSetting 级联编排顺序：分组名 → 渠道 ID 有序列表。
// 与渠道优先级完全解耦：级联选择器按此列表依次尝试；未配置的分组、
// 未入列的渠道按现行优先级降序兜底（首次使用行为与旧版一致）。
// 详见 docs/channel/cascade-failover.md
type CascadeOrderSetting struct {
	GroupOrders map[string][]int `json:"group_orders"`
}

var cascadeOrderSetting = CascadeOrderSetting{
	GroupOrders: map[string][]int{},
}

func init() {
	config.GlobalConfig.Register("cascade_order", &cascadeOrderSetting)
}

func GetCascadeOrderSetting() *CascadeOrderSetting {
	return &cascadeOrderSetting
}

// GetCascadeGroupOrder 返回分组的编排顺序列表；nil/空 = 未配置（走优先级兜底）
func GetCascadeGroupOrder(group string) []int {
	return cascadeOrderSetting.GroupOrders[group]
}
