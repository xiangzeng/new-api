package model

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The cached user snapshot is read back by Go field name, so both paths that
// produce a UserBase must carry every field: the Redis write script and the
// database fallback in GetUserCache. A field dropped from either path reads
// back as its zero value, which silently downgrades authorization and billing
// decisions instead of failing. CustomPricing has been lost that way twice —
// once in the database fallback and once in the Redis write script — each time
// billing per-user pricing users at the plain group ratio.

func TestUserCacheWriteScriptCoversEveryCachedField(t *testing.T) {
	userBaseType := reflect.TypeOf(UserBase{})
	for i := range userBaseType.NumField() {
		field := userBaseType.Field(i)
		assert.Truef(t, strings.Contains(userCacheWriteScript, "'"+field.Name+"'"),
			"UserBase field %s is never written by userCacheWriteScript, so it reads back empty on every cache hit", field.Name)
	}
}

func TestToBaseUserCopiesEveryCachedField(t *testing.T) {
	user := User{
		Id:            7,
		Group:         "svip",
		Email:         "user@example.com",
		Quota:         1234,
		Status:        1,
		Role:          10,
		Username:      "tester",
		Setting:       `{"accept_unset_ratio_model":true}`,
		CustomPricing: `{"enabled":true,"groups":{"kiro":{"ratio":1}}}`,
		AuthVersion:   3,
	}

	base := user.ToBaseUser()
	require.NotNil(t, base)

	baseValue := reflect.ValueOf(*base)
	for i := range baseValue.NumField() {
		field := baseValue.Type().Field(i)
		assert.Falsef(t, baseValue.Field(i).IsZero(),
			"UserBase field %s is not populated by ToBaseUser, so a cache miss loses it", field.Name)
	}
}

func TestUserBaseGetCustomPricingRoundTrip(t *testing.T) {
	base := UserBase{CustomPricing: `{"enabled":true,"groups":{"kiro-high":{"ratio":1}}}`}

	pricing := base.GetCustomPricing()

	require.True(t, pricing.Enabled)
	group, ok := pricing.Groups["kiro-high"]
	require.True(t, ok)
	assert.Equal(t, float64(1), group.Ratio)

	unset := UserBase{}
	assert.False(t, unset.GetCustomPricing().Enabled)
}
