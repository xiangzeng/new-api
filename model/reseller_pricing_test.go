package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupResellerPricingTestDB(t *testing.T) {
	t.Helper()
	previousDB := DB
	previousMainType := common.MainDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.LogDatabaseType())
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ResellerProfile{}, &ResellerCustomer{}, &ResellerPricingRule{}))
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		common.SetDatabaseTypes(previousMainType, common.LogDatabaseType())
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestPlanResellerPricingUpdateActivationRules(t *testing.T) {
	const now = int64(1_700_000_000)

	created, err := PlanResellerPricingUpdate(nil, 15000, now)
	require.NoError(t, err)
	assert.Equal(t, 15000, created.CurrentMultiplierBps)
	assert.Zero(t, created.PendingMultiplierBps)

	increase, err := PlanResellerPricingUpdate(&created, 20000, now)
	require.NoError(t, err)
	assert.Equal(t, 15000, increase.CurrentMultiplierBps)
	assert.Equal(t, 20000, increase.PendingMultiplierBps)
	assert.Equal(t, now+ResellerPriceIncreaseWait, increase.PendingEffectiveAt)
	assert.Equal(t, 15000, ActiveResellerMultiplier(increase, now+ResellerPriceIncreaseWait-1))
	assert.Equal(t, 20000, ActiveResellerMultiplier(increase, now+ResellerPriceIncreaseWait))

	decrease, err := PlanResellerPricingUpdate(&increase, 12000, now+60)
	require.NoError(t, err)
	assert.Equal(t, 12000, decrease.CurrentMultiplierBps)
	assert.Zero(t, decrease.PendingMultiplierBps)
	assert.Zero(t, decrease.PendingEffectiveAt)

	replacement, err := PlanResellerPricingUpdate(&created, 18000, now)
	require.NoError(t, err)
	replacement, err = PlanResellerPricingUpdate(&replacement, 19000, now+300)
	require.NoError(t, err)
	assert.Equal(t, 19000, replacement.PendingMultiplierBps)
	assert.Equal(t, now+300+ResellerPriceIncreaseWait, replacement.PendingEffectiveAt)
}

func TestResolveResellerMultiplierUsesConfirmedPrecedence(t *testing.T) {
	const now = int64(1_700_000_000)
	defaultRules := map[string]ResellerPricingRule{
		"":    {CurrentMultiplierBps: 11000},
		"vip": {CurrentMultiplierBps: 12000},
	}
	customerRules := map[string]ResellerPricingRule{
		"":    {CurrentMultiplierBps: 13000},
		"vip": {CurrentMultiplierBps: 14000},
	}

	tests := []struct {
		name       string
		defaults   map[string]ResellerPricingRule
		customers  map[string]ResellerPricingRule
		group      string
		wantBps    int
		wantSource ResellerMultiplierSource
	}{
		{"customer group", defaultRules, customerRules, "vip", 14000, ResellerMultiplierSourceCustomerGroup},
		{"customer overall", defaultRules, customerRules, "standard", 13000, ResellerMultiplierSourceCustomerOverall},
		{"default group", defaultRules, nil, "vip", 12000, ResellerMultiplierSourceDefaultGroup},
		{"default overall", defaultRules, nil, "standard", 11000, ResellerMultiplierSourceDefaultOverall},
		{"platform base", nil, nil, "standard", 10000, ResellerMultiplierSourceBase},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveResellerMultiplier(tt.defaults, tt.customers, tt.group, now)
			assert.Equal(t, tt.wantBps, got.MultiplierBps)
			assert.Equal(t, tt.wantSource, got.Source)
		})
	}
}

func TestUpdateResellerPricingRuleRejectsStaleVersion(t *testing.T) {
	setupResellerPricingTestDB(t)
	profile := ResellerProfile{UserId: 10, ReceivePublicId: "12345678901234567890123456789012"}
	require.NoError(t, DB.Create(&profile).Error)
	require.EqualValues(t, 1, profile.PricingVersion)

	rule, version, err := UpdateResellerPricingRule(
		ResellerPricingOwnerDefault,
		profile.Id,
		"vip",
		15000,
		1,
		1_700_000_000,
	)
	require.NoError(t, err)
	assert.EqualValues(t, 2, version)
	assert.Equal(t, 15000, rule.CurrentMultiplierBps)

	_, _, err = UpdateResellerPricingRule(
		ResellerPricingOwnerDefault,
		profile.Id,
		"vip",
		16000,
		1,
		1_700_000_100,
	)
	assert.ErrorIs(t, err, ErrResellerPricingVersionConflict)

	var persisted ResellerPricingRule
	require.NoError(t, DB.First(&persisted, rule.Id).Error)
	assert.Equal(t, 15000, persisted.CurrentMultiplierBps)
	assert.Zero(t, persisted.PendingMultiplierBps)
}

func TestUpdateResellerPricingRuleAllowsOneConcurrentWriter(t *testing.T) {
	setupResellerPricingTestDB(t)
	sqlDB, err := DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	profile := ResellerProfile{UserId: 11, ReceivePublicId: "abcdefghijklmnopqrstuvwx12345678"}
	require.NoError(t, DB.Create(&profile).Error)

	start := make(chan struct{})
	errorsByWriter := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errorsByWriter {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()
			<-start
			_, _, errorsByWriter[writer] = UpdateResellerPricingRule(
				ResellerPricingOwnerDefault,
				profile.Id,
				"vip",
				15000+writer*1000,
				1,
				1_700_000_000,
			)
		}(i)
	}
	close(start)
	wg.Wait()

	successCount := 0
	conflictCount := 0
	for _, updateErr := range errorsByWriter {
		if updateErr == nil {
			successCount++
		} else if errors.Is(updateErr, ErrResellerPricingVersionConflict) {
			conflictCount++
		}
	}
	assert.Equal(t, 1, successCount)
	assert.Equal(t, 1, conflictCount)

	var persisted ResellerProfile
	require.NoError(t, DB.First(&persisted, profile.Id).Error)
	assert.EqualValues(t, 2, persisted.PricingVersion)
}

func TestResellerCustomerAndPricingScopeAreUnique(t *testing.T) {
	setupResellerPricingTestDB(t)
	first := ResellerCustomer{ResellerId: 1, CustomerId: 20, RegistrationSource: ResellerRegistrationSourceReseller, BoundAt: 1}
	require.NoError(t, DB.Create(&first).Error)
	duplicate := ResellerCustomer{ResellerId: 2, CustomerId: 20, RegistrationSource: ResellerRegistrationSourceReseller, BoundAt: 2}
	assert.Error(t, DB.Create(&duplicate).Error)

	rule := ResellerPricingRule{OwnerType: ResellerPricingOwnerCustomer, OwnerId: first.Id, GroupName: "vip", CurrentMultiplierBps: 12000}
	require.NoError(t, DB.Create(&rule).Error)
	duplicateRule := rule
	duplicateRule.Id = 0
	assert.Error(t, DB.Create(&duplicateRule).Error)
}

func TestValidateResellerMultiplierBps(t *testing.T) {
	tests := []struct {
		value int
		valid bool
	}{
		{9999, false},
		{10000, true},
		{100000, true},
		{100001, false},
	}
	for _, tt := range tests {
		err := ValidateResellerMultiplierBps(tt.value)
		if tt.valid {
			require.NoError(t, err)
		} else {
			assert.True(t, errors.Is(err, ErrResellerMultiplierOutOfRange))
		}
	}
}
