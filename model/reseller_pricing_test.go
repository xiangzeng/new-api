package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

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
	require.NoError(t, db.AutoMigrate(
		&ResellerProfile{}, &ResellerCustomer{}, &ResellerPricingRule{}, &ResellerCommissionEntry{},
		&ResellerLedgerTransaction{}, &ResellerLedgerLine{},
	))
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

func TestResolveActiveResellerPricingUsesBindingAndRuleScopes(t *testing.T) {
	setupResellerPricingTestDB(t)
	profile := ResellerProfile{UserId: 41, ReceivePublicId: "resolve-active-reseller-pricing01"}
	require.NoError(t, DB.Create(&profile).Error)
	binding := ResellerCustomer{ResellerId: 41, CustomerId: 42, RegistrationSource: ResellerRegistrationSourceReseller, BoundAt: 1}
	require.NoError(t, DB.Create(&binding).Error)
	require.NoError(t, DB.Create(&ResellerPricingRule{
		OwnerType: ResellerPricingOwnerDefault, OwnerId: profile.Id, GroupName: "vip", CurrentMultiplierBps: 12000,
	}).Error)
	require.NoError(t, DB.Create(&ResellerPricingRule{
		OwnerType: ResellerPricingOwnerCustomer, OwnerId: binding.Id, GroupName: "vip", CurrentMultiplierBps: 14500,
	}).Error)

	pricing, err := ResolveActiveResellerPricing(42, "vip", 1_700_000_000)
	require.NoError(t, err)
	require.NotNil(t, pricing)
	assert.Equal(t, 41, pricing.ResellerId)
	assert.EqualValues(t, binding.Id, pricing.CustomerBindingId)
	assert.Equal(t, 14500, pricing.MultiplierBps)
	assert.Equal(t, ResellerMultiplierSourceCustomerGroup, pricing.MultiplierSource)

	require.NoError(t, DB.Model(&ResellerProfile{}).Where("id = ?", profile.Id).Update("status", ResellerStatusFrozen).Error)
	pricing, err = ResolveActiveResellerPricing(42, "vip", 1_700_000_000)
	require.NoError(t, err)
	assert.Nil(t, pricing)
}

func TestCreateResellerCommissionIsConcurrentAndReplaySafe(t *testing.T) {
	setupResellerPricingTestDB(t)
	sqlDB, err := DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	profile := ResellerProfile{UserId: 51, ReceivePublicId: "commission-concurrent-profile-01"}
	require.NoError(t, DB.Create(&profile).Error)

	params := CreateResellerCommissionParams{
		RequestReference:  "request:req-phase3:final",
		ResellerId:        51,
		CustomerId:        52,
		CustomerBindingId: 53,
		MultiplierBps:     12500,
		MultiplierSource:  string(ResellerMultiplierSourceCustomerOverall),
		BaseQuota:         101,
		RetailQuota:       127,
		Now:               time.Date(2026, 8, 4, 3, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)),
	}

	const workers = 8
	entries := make([]*ResellerCommissionEntry, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			entries[index], errs[index] = CreateResellerCommission(params)
		}(i)
	}
	wg.Wait()
	for i := range workers {
		require.NoError(t, errs[i])
		require.NotNil(t, entries[i])
		assert.Equal(t, 26, entries[i].CommissionQuota)
	}
	var count int64
	require.NoError(t, DB.Model(&ResellerCommissionEntry{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, DB.Model(&ResellerLedgerTransaction{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	var lines []ResellerLedgerLine
	require.NoError(t, DB.Order("line_number asc").Find(&lines).Error)
	require.Len(t, lines, 2)
	assert.EqualValues(t, 0, lines[0].DeltaQuota+lines[1].DeltaQuota)
	require.NoError(t, DB.First(&profile, profile.Id).Error)
	assert.EqualValues(t, 26, profile.PendingCommissionQuota)
	assert.Zero(t, profile.AvailableCommissionQuota)

	conflict := params
	conflict.RetailQuota++
	_, err = CreateResellerCommission(conflict)
	assert.ErrorIs(t, err, ErrResellerCommissionReferenceConflict)
}

func TestReleaseDueResellerCommissionsMovesBalancedProjectionOnce(t *testing.T) {
	setupResellerPricingTestDB(t)
	profile := ResellerProfile{UserId: 61, ReceivePublicId: "release-commission-profile-0001"}
	require.NoError(t, DB.Create(&profile).Error)
	now := time.Date(2026, 8, 4, 5, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	entry, err := CreateResellerCommission(CreateResellerCommissionParams{
		RequestReference: "request:release-once:final", ResellerId: 61, CustomerId: 62, CustomerBindingId: 63,
		MultiplierBps: 13000, MultiplierSource: string(ResellerMultiplierSourceDefaultOverall),
		BaseQuota: 100, RetailQuota: 130, Now: now,
	})
	require.NoError(t, err)
	require.NotNil(t, entry)

	batch, err := ReleaseDueResellerCommissionsBatch(t.Context(), entry.ReleaseAt, 50)
	require.NoError(t, err)
	assert.Equal(t, 1, batch.Processed)
	assert.Zero(t, batch.Remaining)

	require.NoError(t, DB.First(entry, entry.Id).Error)
	assert.Equal(t, ResellerCommissionStatusAvailable, entry.Status)
	require.NoError(t, DB.First(&profile, profile.Id).Error)
	assert.Zero(t, profile.PendingCommissionQuota)
	assert.EqualValues(t, 30, profile.AvailableCommissionQuota)

	var journals []ResellerLedgerTransaction
	require.NoError(t, DB.Order("id asc").Find(&journals).Error)
	require.Len(t, journals, 2)
	for _, journal := range journals {
		var lines []ResellerLedgerLine
		require.NoError(t, DB.Where("transaction_id = ?", journal.Id).Find(&lines).Error)
		var sum int64
		for _, line := range lines {
			sum += line.DeltaQuota
		}
		assert.Zero(t, sum)
	}

	replayed, err := ReleaseDueResellerCommissionsBatch(t.Context(), entry.ReleaseAt+60, 50)
	require.NoError(t, err)
	assert.Zero(t, replayed.Processed)
	var journalCount int64
	require.NoError(t, DB.Model(&ResellerLedgerTransaction{}).Count(&journalCount).Error)
	assert.EqualValues(t, 2, journalCount)
}

func TestReleaseDueResellerCommissionRollsBackOnProjectionMismatch(t *testing.T) {
	setupResellerPricingTestDB(t)
	profile := ResellerProfile{UserId: 71, ReceivePublicId: "release-rollback-profile-00001"}
	require.NoError(t, DB.Create(&profile).Error)
	entry, err := CreateResellerCommission(CreateResellerCommissionParams{
		RequestReference: "request:release-rollback:final", ResellerId: 71, CustomerId: 72, CustomerBindingId: 73,
		MultiplierBps: 14000, MultiplierSource: string(ResellerMultiplierSourceCustomerOverall),
		BaseQuota: 100, RetailQuota: 140, Now: time.Unix(1_700_000_000, 0),
	})
	require.NoError(t, err)
	require.NoError(t, DB.Model(&ResellerProfile{}).Where("id = ?", profile.Id).Update("pending_commission_quota", 0).Error)

	_, err = ReleaseDueResellerCommissionsBatch(t.Context(), entry.ReleaseAt, 50)
	assert.ErrorIs(t, err, ErrResellerBalanceProjection)
	require.NoError(t, DB.First(entry, entry.Id).Error)
	assert.Equal(t, ResellerCommissionStatusPending, entry.Status)
	var journalCount int64
	require.NoError(t, DB.Model(&ResellerLedgerTransaction{}).Count(&journalCount).Error)
	assert.EqualValues(t, 1, journalCount, "release journal must roll back with projection failure")
}

func TestValidateResellerLedgerLinesRejectsUnbalancedOrOverflow(t *testing.T) {
	assert.ErrorIs(t, validateResellerLedgerLines([]ResellerLedgerLineInput{
		{Account: "a", DeltaQuota: -10}, {Account: "b", DeltaQuota: 9},
	}), ErrResellerLedgerUnbalanced)
	assert.ErrorIs(t, validateResellerLedgerLines([]ResellerLedgerLineInput{
		{Account: "a", DeltaQuota: int64(^uint64(0) >> 1)}, {Account: "b", DeltaQuota: 1},
	}), ErrResellerLedgerUnbalanced)
}

func TestNextResellerCommissionReleaseAtUsesBeijing0410Boundary(t *testing.T) {
	beijing := time.FixedZone("Asia/Shanghai", 8*60*60)
	before := time.Date(2026, 8, 4, 4, 9, 59, 0, beijing)
	atBoundary := time.Date(2026, 8, 4, 4, 10, 0, 0, beijing)

	assert.Equal(t, time.Date(2026, 8, 4, 4, 10, 0, 0, beijing).Unix(), NextResellerCommissionReleaseAt(before))
	assert.Equal(t, time.Date(2026, 8, 5, 4, 10, 0, 0, beijing).Unix(), NextResellerCommissionReleaseAt(atBoundary))
}
