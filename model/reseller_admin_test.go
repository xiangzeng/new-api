package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupResellerAdminTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousMainType := common.MainDatabaseType()
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&User{}, &ResellerProfile{}, &ResellerCustomer{}, &ResellerPricingRule{}, &ResellerCommissionEntry{},
	))
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousMainType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func createResellerAdminUser(t *testing.T, db *gorm.DB, username string) User {
	t.Helper()
	user := User{
		Username: username,
		Password: "unused",
		AffCode:  "aff-" + username,
		Group:    "default",
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func TestAdminBindOpensResellerCenterAndSyncsInviter(t *testing.T) {
	db := setupResellerAdminTestDB(t)
	reseller := createResellerAdminUser(t, db, "admin-bind-reseller")
	customer := createResellerAdminUser(t, db, "admin-bind-customer")

	binding, err := AdminBindResellerCustomer(reseller.Id, "", customer.Id, 1_700_000_000)
	require.NoError(t, err)
	assert.Equal(t, reseller.Id, binding.ResellerId)
	assert.Equal(t, customer.Id, binding.CustomerId)
	assert.Equal(t, ResellerRegistrationSourceAdmin, binding.RegistrationSource)
	assert.Equal(t, int64(1_700_000_000), binding.BoundAt)

	var profile ResellerProfile
	require.NoError(t, db.Where("user_id = ?", reseller.Id).First(&profile).Error)
	assert.Equal(t, ResellerStatusActive, profile.Status)
	assert.Len(t, profile.ReceivePublicId, 32)

	var stored User
	require.NoError(t, db.First(&stored, customer.Id).Error)
	assert.Equal(t, reseller.Id, stored.InviterId)

	repeat, err := AdminBindResellerCustomer(0, reseller.Username, customer.Id, 1_700_000_500)
	require.NoError(t, err)
	assert.Equal(t, binding.Id, repeat.Id)
	assert.Equal(t, int64(1_700_000_000), repeat.BoundAt)
}

func TestAdminBindRejectsSelfAndForeignOwnership(t *testing.T) {
	db := setupResellerAdminTestDB(t)
	firstReseller := createResellerAdminUser(t, db, "admin-first-reseller")
	secondReseller := createResellerAdminUser(t, db, "admin-second-reseller")
	customer := createResellerAdminUser(t, db, "admin-owned-customer")

	_, err := AdminBindResellerCustomer(firstReseller.Id, "", firstReseller.Id, 1_700_000_000)
	assert.ErrorIs(t, err, ErrResellerSelfBinding)

	_, err = AdminBindResellerCustomer(firstReseller.Id, "", customer.Id, 1_700_000_000)
	require.NoError(t, err)

	_, err = AdminBindResellerCustomer(secondReseller.Id, "", customer.Id, 1_700_000_100)
	assert.ErrorIs(t, err, ErrResellerCustomerBound)

	var binding ResellerCustomer
	require.NoError(t, db.Where("customer_id = ?", customer.Id).First(&binding).Error)
	assert.Equal(t, firstReseller.Id, binding.ResellerId)

	var count int64
	require.NoError(t, db.Model(&ResellerProfile{}).Where("user_id = ?", secondReseller.Id).Count(&count).Error)
	assert.Zero(t, count, "a rejected binding must not open the reseller center")
}

func TestAdminUnbindClearsPricingAndKeepsCommissionHistory(t *testing.T) {
	db := setupResellerAdminTestDB(t)
	reseller := createResellerAdminUser(t, db, "admin-unbind-reseller")
	customer := createResellerAdminUser(t, db, "admin-unbind-customer")

	binding, err := AdminBindResellerCustomer(reseller.Id, "", customer.Id, 1_700_000_000)
	require.NoError(t, err)
	require.NoError(t, db.Create(&ResellerPricingRule{
		OwnerType: ResellerPricingOwnerCustomer, OwnerId: binding.Id,
		CurrentMultiplierBps: 15000, Version: 1,
	}).Error)
	require.NoError(t, db.Create(&ResellerCommissionEntry{
		RequestReference: "request:admin-unbind:final", ResellerId: reseller.Id, CustomerId: customer.Id,
		CustomerBindingId: binding.Id, MultiplierBps: 15000, MultiplierSource: string(ResellerMultiplierSourceCustomerOverall),
		BaseQuota: 100, RetailQuota: 150, CommissionQuota: 50, Status: ResellerCommissionStatusPending,
	}).Error)

	require.NoError(t, AdminUnbindResellerCustomer(customer.Id))

	var bindingCount, ruleCount, commissionCount int64
	require.NoError(t, db.Model(&ResellerCustomer{}).Where("customer_id = ?", customer.Id).Count(&bindingCount).Error)
	assert.Zero(t, bindingCount)
	require.NoError(t, db.Model(&ResellerPricingRule{}).
		Where("owner_type = ? AND owner_id = ?", ResellerPricingOwnerCustomer, binding.Id).Count(&ruleCount).Error)
	assert.Zero(t, ruleCount)
	require.NoError(t, db.Model(&ResellerCommissionEntry{}).Where("customer_id = ?", customer.Id).Count(&commissionCount).Error)
	assert.Equal(t, int64(1), commissionCount)

	var stored User
	require.NoError(t, db.First(&stored, customer.Id).Error)
	assert.Zero(t, stored.InviterId)

	rebound, err := AdminBindResellerCustomer(reseller.Id, "", customer.Id, 1_700_001_000)
	require.NoError(t, err)
	assert.Equal(t, int64(1_700_001_000), rebound.BoundAt)
	// A rebound customer must start from inherited default pricing. Databases
	// are free to hand out the released binding id again, so a stale rule left
	// behind by the unbind would silently resurrect the old multiplier.
	require.NoError(t, db.Model(&ResellerPricingRule{}).
		Where("owner_type = ? AND owner_id = ?", ResellerPricingOwnerCustomer, rebound.Id).Count(&ruleCount).Error)
	assert.Zero(t, ruleCount)
}

func TestAdminGetResellerBindingReportsOwnershipAndRole(t *testing.T) {
	db := setupResellerAdminTestDB(t)
	reseller := createResellerAdminUser(t, db, "admin-view-reseller")
	customer := createResellerAdminUser(t, db, "admin-view-customer")

	unbound, err := AdminGetResellerBinding(customer.Id, 1_700_000_000)
	require.NoError(t, err)
	assert.False(t, unbound.Bound)
	assert.False(t, unbound.IsReseller)
	assert.Equal(t, customer.Username, unbound.CustomerUsername)

	binding, err := AdminBindResellerCustomer(reseller.Id, "", customer.Id, 1_700_000_000)
	require.NoError(t, err)
	var profile ResellerProfile
	require.NoError(t, db.Where("user_id = ?", reseller.Id).First(&profile).Error)
	require.NoError(t, db.Create(&ResellerPricingRule{
		OwnerType: ResellerPricingOwnerCustomer, OwnerId: binding.Id,
		CurrentMultiplierBps: 12000, Version: 1,
	}).Error)

	bound, err := AdminGetResellerBinding(customer.Id, 1_700_000_100)
	require.NoError(t, err)
	assert.True(t, bound.Bound)
	assert.Equal(t, binding.Id, bound.BindingId)
	assert.Equal(t, reseller.Id, bound.ResellerId)
	assert.Equal(t, reseller.Username, bound.ResellerUsername)
	assert.Equal(t, ResellerStatusActive, bound.ResellerStatus)
	assert.Equal(t, ResellerRegistrationSourceAdmin, bound.RegistrationSource)
	assert.Equal(t, 12000, bound.CurrentMultiplierBps)
	assert.Equal(t, string(ResellerMultiplierSourceCustomerOverall), bound.MultiplierSource)

	resellerView, err := AdminGetResellerBinding(reseller.Id, 1_700_000_100)
	require.NoError(t, err)
	assert.True(t, resellerView.IsReseller)
	assert.Equal(t, int64(1), resellerView.OwnCustomerCount)
	assert.False(t, resellerView.Bound)
}
