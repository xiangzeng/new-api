package system_setting

import "github.com/QuantumNous/new-api/setting/config"

// OpenBalanceApiSettings controls the third-party balance query API.
//
// Rate limits are keyed by app and by (app, username) rather than by client IP:
// partners authenticate with a secret that cannot live in a browser, so every
// request arrives from the partner's own server and an IP-keyed limit would
// throttle all of their users as one client.
type OpenBalanceApiSettings struct {
	Enabled bool `json:"enabled"`
	// ExchangeRateLimitPerMinute caps credential exchanges per app per minute.
	// A per-app override on the app row takes precedence when non-zero.
	ExchangeRateLimitPerMinute int `json:"exchange_rate_limit_per_minute"`
	// ExchangeIpRateLimitPerMinute is the pre-authentication backstop on the
	// exchange endpoint, keyed by source IP. It exists to bound anonymous
	// floods that would otherwise reach the app lookup, and is set well above
	// what a real partner needs because exchanges happen once per user.
	ExchangeIpRateLimitPerMinute int `json:"exchange_ip_rate_limit_per_minute"`
	// BalanceRateLimitPerMinute caps balance reads per credential per minute.
	BalanceRateLimitPerMinute int `json:"balance_rate_limit_per_minute"`
	// FailureLockThreshold is how many consecutive failed password attempts for
	// one (app, username) pair trigger a lockout. This is the primary defense
	// against credential stuffing through a partner.
	FailureLockThreshold int `json:"failure_lock_threshold"`
	// FailureLockMinutes is how long that lockout lasts.
	FailureLockMinutes int `json:"failure_lock_minutes"`
}

var defaultOpenBalanceApiSettings = OpenBalanceApiSettings{
	Enabled:                      false,
	ExchangeRateLimitPerMinute:   300,
	ExchangeIpRateLimitPerMinute: 600,
	BalanceRateLimitPerMinute:    120,
	FailureLockThreshold:         5,
	FailureLockMinutes:           15,
}

func init() {
	config.GlobalConfig.Register("open_balance_api", &defaultOpenBalanceApiSettings)
}

func GetOpenBalanceApiSettings() *OpenBalanceApiSettings {
	return &defaultOpenBalanceApiSettings
}
