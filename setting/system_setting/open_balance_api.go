package system_setting

import "github.com/QuantumNous/new-api/setting/config"

// OpenBalanceApiSettings controls the self-service balance query API.
//
// The limit is keyed by balance key rather than by client IP: a user may read
// one key from several machines, and several users may sit behind one address,
// so an IP-keyed limit would both leak across accounts and throttle a single
// legitimate caller inconsistently.
type OpenBalanceApiSettings struct {
	// BalanceRateLimitPerMinute caps balance reads per key per minute.
	BalanceRateLimitPerMinute int `json:"balance_rate_limit_per_minute"`
}

var defaultOpenBalanceApiSettings = OpenBalanceApiSettings{
	BalanceRateLimitPerMinute: 120,
}

func init() {
	config.GlobalConfig.Register("open_balance_api", &defaultOpenBalanceApiSettings)
}

func GetOpenBalanceApiSettings() *OpenBalanceApiSettings {
	return &defaultOpenBalanceApiSettings
}
