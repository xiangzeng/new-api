package dto

type UserCustomPricing struct {
	Enabled bool                        `json:"enabled"`
	Groups  map[string]UserGroupPricing `json:"groups,omitempty"`
}

type UserGroupPricing struct {
	Ratio float64 `json:"ratio"`
}
