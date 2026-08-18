package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// CascadeSetting 渠道级联与熔断恢复配置。
// 详见 docs/channel/cascade-failover.md
type CascadeSetting struct {
	Enabled               bool `json:"enabled"`
	FailureThreshold      int  `json:"failure_threshold"`
	CooldownSeconds       int  `json:"cooldown_seconds"`
	ProbeEnabled          bool `json:"probe_enabled"`
	ProbeIntervalSeconds  int  `json:"probe_interval_seconds"`
	RecoverySuccessCount  int  `json:"recovery_success_count"`
	MaxAttemptsPerRequest int  `json:"max_attempts_per_request"`
	// RPM 水位线总开关：关闭时 RPM 照常统计（编排页可见），但不参与选路，
	// 便于先观察真实 RPM 再决定水位线数值。水位线配置见 cascade_watermark。
	WatermarkEnabled bool `json:"watermark_enabled"`
	// 流以 EOF 结束但未收到协议完成标记（上游安静断流）时是否计入渠道故障。
	// 仅对支持完成标记跟踪的适配器生效（当前为 Claude SSE）。
	IncompleteStreamAsFault bool `json:"incomplete_stream_as_fault"`
	// 白名单：额外视为渠道故障的状态码（应对把渠道故障报成 400 类的上游）
	ExtraFaultStatusCodes []int `json:"extra_fault_status_codes"`
	// 白名单：错误内容命中关键词即视为渠道故障
	ExtraFaultKeywords []string `json:"extra_fault_keywords"`
	// 排除：错误内容命中关键词即不视为渠道故障（优先级最高）
	IgnoreFaultKeywords []string `json:"ignore_fault_keywords"`
}

var cascadeSetting = CascadeSetting{
	Enabled:                 false,
	FailureThreshold:        2,
	CooldownSeconds:         120,
	ProbeEnabled:            true,
	ProbeIntervalSeconds:    60,
	RecoverySuccessCount:    3,
	MaxAttemptsPerRequest:   0,
	WatermarkEnabled:        false,
	IncompleteStreamAsFault: true,
	ExtraFaultStatusCodes:   []int{},
	ExtraFaultKeywords:      []string{},
	IgnoreFaultKeywords:     []string{},
}

func init() {
	config.GlobalConfig.Register("cascade_setting", &cascadeSetting)
}

func GetCascadeSetting() *CascadeSetting {
	return &cascadeSetting
}

// GetFailureThreshold 返回合法化后的熔断触发阈值（至少 1 次）
func (s *CascadeSetting) GetFailureThreshold() int {
	if s.FailureThreshold < 1 {
		return 1
	}
	return s.FailureThreshold
}

// GetCooldownSeconds 返回合法化后的冷却时长
func (s *CascadeSetting) GetCooldownSeconds() int {
	if s.CooldownSeconds < 1 {
		return 120
	}
	return s.CooldownSeconds
}

// GetProbeIntervalSeconds 返回合法化后的探活间隔
func (s *CascadeSetting) GetProbeIntervalSeconds() int {
	if s.ProbeIntervalSeconds < 10 {
		return 60
	}
	return s.ProbeIntervalSeconds
}

// GetRecoverySuccessCount 返回合法化后的恢复门槛（至少 1 次）
func (s *CascadeSetting) GetRecoverySuccessCount() int {
	if s.RecoverySuccessCount < 1 {
		return 3
	}
	return s.RecoverySuccessCount
}
