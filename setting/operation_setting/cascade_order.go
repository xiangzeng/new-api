package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// CascadeOrderSetting 级联编排顺序：分组名 → 渠道 ID 有序列表。
// 与渠道优先级完全解耦：级联选择器按此列表依次尝试；未配置的分组、
// 未入列的渠道按现行优先级降序兜底（首次使用行为与旧版一致）。
// 详见 docs/channel/cascade-failover.md
type CascadeOrderSetting struct {
	GroupOrders map[string][]int `json:"group_orders"`
	// GroupSequence 分组泳道在编排页的展示顺序（组名有序列表）。纯展示，不参与路由；
	// 未入列的分组按组名升序垫底，失效（孤儿）分组一律沉底。
	GroupSequence []string `json:"group_sequence"`
}

var cascadeOrderSetting = CascadeOrderSetting{
	GroupOrders:   map[string][]int{},
	GroupSequence: []string{},
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

// GetCascadeGroupSequence 返回分组在编排页的展示顺序；nil/空 = 未配置（按组名升序兜底）
func GetCascadeGroupSequence() []string {
	return cascadeOrderSetting.GroupSequence
}

// GetCascadeGroupSequencePositions 组名 → 1 基位置；0 表示未入列（垫底）
func GetCascadeGroupSequencePositions() map[string]int {
	positions := make(map[string]int, len(cascadeOrderSetting.GroupSequence))
	for index, name := range cascadeOrderSetting.GroupSequence {
		if _, ok := positions[name]; ok {
			continue
		}
		positions[name] = index + 1
	}
	return positions
}
