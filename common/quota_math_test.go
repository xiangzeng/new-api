package common

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

const overflowingProduct = 2000 * 1.8446744073686647e19

func TestQuotaFromFloat(t *testing.T) {
	assert.Equal(t, 42, QuotaFromFloat(42.4))
	assert.Equal(t, 42, QuotaFromFloat(42.9))
	assert.Equal(t, -42, QuotaFromFloat(-42.9))
	assert.Equal(t, MaxQuota, QuotaFromFloat(overflowingProduct))
	assert.Equal(t, MinQuota, QuotaFromFloat(-overflowingProduct))
	assert.Equal(t, MaxQuota, QuotaFromFloat(math.Inf(1)))
	assert.Equal(t, MinQuota, QuotaFromFloat(math.Inf(-1)))
	assert.Equal(t, 0, QuotaFromFloat(math.NaN()))
}

func TestQuotaRound(t *testing.T) {
	assert.Equal(t, 42, QuotaRound(41.5))
	assert.Equal(t, 43, QuotaRound(42.5))
	assert.Equal(t, -43, QuotaRound(-42.5))
	assert.Equal(t, MaxQuota, QuotaRound(overflowingProduct))
	assert.Equal(t, MinQuota, QuotaRound(-overflowingProduct))
	assert.Equal(t, 0, QuotaRound(math.NaN()))
}

func TestQuotaFromDecimal(t *testing.T) {
	assert.Equal(t, 43, QuotaFromDecimal(decimal.NewFromFloat(42.5)))
	assert.Equal(t, 42, QuotaFromDecimal(decimal.NewFromFloat(41.7)))
	assert.Equal(t, MaxQuota, QuotaFromDecimal(decimal.NewFromInt(2000).Mul(decimal.NewFromFloat(1.8446744073686647e19))))
	assert.Equal(t, MinQuota, QuotaFromDecimal(decimal.NewFromInt(-2000).Mul(decimal.NewFromFloat(1.8446744073686647e19))))
}

func TestQuotaFromFloatChecked(t *testing.T) {
	quota, clamp := QuotaFromFloatChecked(42.9)
	assert.Equal(t, 42, quota)
	assert.Nil(t, clamp)

	quota, clamp = QuotaFromFloatChecked(overflowingProduct)
	assert.Equal(t, MaxQuota, quota)
	if assert.NotNil(t, clamp) {
		assert.Equal(t, "QuotaFromFloat", clamp.Op)
		assert.Equal(t, QuotaClampOverflow, clamp.Kind)
		assert.Equal(t, MaxQuota, clamp.Clamped)
	}

	quota, clamp = QuotaFromFloatChecked(-overflowingProduct)
	assert.Equal(t, MinQuota, quota)
	if assert.NotNil(t, clamp) {
		assert.Equal(t, QuotaClampUnderflow, clamp.Kind)
		assert.Equal(t, MinQuota, clamp.Clamped)
	}

	quota, clamp = QuotaFromFloatChecked(math.NaN())
	assert.Equal(t, 0, quota)
	if assert.NotNil(t, clamp) {
		assert.Equal(t, QuotaClampNaN, clamp.Kind)
		assert.Equal(t, 0, clamp.Clamped)
	}
}

func TestQuotaRoundChecked(t *testing.T) {
	quota, clamp := QuotaRoundChecked(42.5)
	assert.Equal(t, 43, quota)
	assert.Nil(t, clamp)

	quota, clamp = QuotaRoundChecked(overflowingProduct)
	assert.Equal(t, MaxQuota, quota)
	if assert.NotNil(t, clamp) {
		assert.Equal(t, "QuotaRound", clamp.Op)
		assert.Equal(t, QuotaClampOverflow, clamp.Kind)
	}
}

func TestQuotaFromDecimalChecked(t *testing.T) {
	quota, clamp := QuotaFromDecimalChecked(decimal.NewFromFloat(41.7))
	assert.Equal(t, 42, quota)
	assert.Nil(t, clamp)

	quota, clamp = QuotaFromDecimalChecked(decimal.NewFromInt(2000).Mul(decimal.NewFromFloat(1.8446744073686647e19)))
	assert.Equal(t, MaxQuota, quota)
	if assert.NotNil(t, clamp) {
		assert.Equal(t, "QuotaFromDecimal", clamp.Op)
		assert.Equal(t, QuotaClampOverflow, clamp.Kind)
	}
}
