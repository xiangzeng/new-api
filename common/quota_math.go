package common

import (
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

const (
	MaxQuota = math.MaxInt32
	MinQuota = math.MinInt32
)

const (
	QuotaClampOverflow  = "overflow"
	QuotaClampUnderflow = "underflow"
	QuotaClampNaN       = "nan"
)

type QuotaClamp struct {
	Op       string  `json:"op"`
	Kind     string  `json:"kind"`
	Original float64 `json:"original"`
	Clamped  int     `json:"clamped"`
}

func (c *QuotaClamp) AuditMap() map[string]interface{} {
	if c == nil {
		return nil
	}
	return map[string]interface{}{
		"op":       c.Op,
		"kind":     c.Kind,
		"original": c.Original,
		"clamped":  c.Clamped,
	}
}

func saturateQuota(value float64, op string) (int, *QuotaClamp) {
	switch {
	case math.IsNaN(value):
		SysError(fmt.Sprintf("quota conversion (%s) received NaN, falling back to 0", op))
		return 0, &QuotaClamp{Op: op, Kind: QuotaClampNaN, Original: value, Clamped: 0}
	case value >= MaxQuota:
		SysError(fmt.Sprintf("quota conversion (%s) overflow: %g exceeds max quota, clamped to %d", op, value, MaxQuota))
		return MaxQuota, &QuotaClamp{Op: op, Kind: QuotaClampOverflow, Original: value, Clamped: MaxQuota}
	case value <= MinQuota:
		SysError(fmt.Sprintf("quota conversion (%s) underflow: %g below min quota, clamped to %d", op, value, MinQuota))
		return MinQuota, &QuotaClamp{Op: op, Kind: QuotaClampUnderflow, Original: value, Clamped: MinQuota}
	default:
		return int(value), nil
	}
}

func QuotaFromFloat(value float64) int {
	quota, _ := QuotaFromFloatChecked(value)
	return quota
}

func QuotaFromFloatChecked(value float64) (int, *QuotaClamp) {
	return saturateQuota(value, "QuotaFromFloat")
}

func QuotaRound(value float64) int {
	quota, _ := QuotaRoundChecked(value)
	return quota
}

func QuotaRoundChecked(value float64) (int, *QuotaClamp) {
	return saturateQuota(math.Round(value), "QuotaRound")
}

func QuotaFromDecimal(d decimal.Decimal) int {
	quota, _ := QuotaFromDecimalChecked(d)
	return quota
}

func QuotaFromDecimalChecked(d decimal.Decimal) (int, *QuotaClamp) {
	f, _ := d.Round(0).Float64()
	return saturateQuota(f, "QuotaFromDecimal")
}
