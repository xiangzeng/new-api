package dto

type UserCustomPricing struct {
	Enabled     bool                        `json:"enabled"`
	Groups      map[string]UserGroupPricing `json:"groups,omitempty"`
	ExtraGroups map[string]string           `json:"extra_groups,omitempty"` // 额外可见分组 {分组名: 描述}
	HideGroups  []string                    `json:"hide_groups,omitempty"` // 强制隐藏的分组
}

type UserGroupPricing struct {
	Ratio float64 `json:"ratio"`
}
