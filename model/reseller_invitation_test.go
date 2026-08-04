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

func setupResellerInvitationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousSecret := common.SessionSecret
	previousMainType := common.MainDatabaseType()
	common.SessionSecret = "reseller-invitation-test-secret"
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &ResellerProfile{}, &ResellerCustomer{}, &ResellerInvitation{}))
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		common.SessionSecret = previousSecret
		common.SetMainDatabaseType(previousMainType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func createResellerInvitationFixture(t *testing.T, db *gorm.DB, username string) (User, ResellerProfile, string) {
	t.Helper()
	reseller := User{
		Username: username,
		Password: "unused",
		AffCode:  "aff-" + username,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}
	require.NoError(t, db.Create(&reseller).Error)
	profile := ResellerProfile{
		UserId:          reseller.Id,
		ReceivePublicId: fmt.Sprintf("%032d", reseller.Id),
	}
	require.NoError(t, db.Create(&profile).Error)
	token, _, err := GetOrCreateResellerInvitation(reseller.Id, 1_700_000_000)
	require.NoError(t, err)
	return reseller, profile, token
}

func TestResellerInvitationIsOpaqueStableAndNotPersistedRaw(t *testing.T) {
	db := setupResellerInvitationTestDB(t)
	reseller, _, token := createResellerInvitationFixture(t, db, "opaque-reseller")
	assert.True(t, strings.HasPrefix(token, "i1."))

	secondToken, invitation, err := GetOrCreateResellerInvitation(reseller.Id, 1_700_000_100)
	require.NoError(t, err)
	assert.Equal(t, token, secondToken)
	assert.NotEqual(t, token, invitation.TokenHash)
	assert.NotContains(t, invitation.TokenHash, invitation.PublicId)
	assert.Equal(t, resellerInvitationTokenHash(token), invitation.TokenHash)

	resolved, err := ResolveResellerInvitation(token, 1_700_000_100)
	require.NoError(t, err)
	assert.Equal(t, reseller.Id, resolved.ResellerId)

	tampered := token[:len(token)-1] + "x"
	_, err = ResolveResellerInvitation(tampered, 1_700_000_100)
	assert.ErrorIs(t, err, ErrResellerInvitationInvalid)
}

func TestResellerCustomerBindingIsAtomicAndImmutable(t *testing.T) {
	db := setupResellerInvitationTestDB(t)
	firstReseller, _, firstToken := createResellerInvitationFixture(t, db, "first-reseller")
	_, _, secondToken := createResellerInvitationFixture(t, db, "second-reseller")

	customer := User{Username: "direct-customer", Password: "unused", AffCode: "aff-direct-customer", Status: common.UserStatusEnabled}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		customer.InviterId = firstReseller.Id
		if err := tx.Create(&customer).Error; err != nil {
			return err
		}
		_, err := BindResellerCustomerFromInvitationWithTx(
			tx,
			firstToken,
			customer.Id,
			ResellerRegistrationSourceReseller,
			1_700_000_000,
		)
		return err
	}))

	var binding ResellerCustomer
	require.NoError(t, db.Where("customer_id = ?", customer.Id).First(&binding).Error)
	assert.Equal(t, firstReseller.Id, binding.ResellerId)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, bindErr := BindResellerCustomerFromInvitationWithTx(
			tx,
			secondToken,
			customer.Id,
			ResellerRegistrationSourceReseller,
			1_700_000_100,
		)
		return bindErr
	})
	assert.ErrorIs(t, err, ErrResellerCustomerBound)
	require.NoError(t, db.Where("customer_id = ?", customer.Id).First(&binding).Error)
	assert.Equal(t, firstReseller.Id, binding.ResellerId)
}

func TestInvalidInvitationRollsBackUserCreation(t *testing.T) {
	db := setupResellerInvitationTestDB(t)
	err := db.Transaction(func(tx *gorm.DB) error {
		customer := User{Username: "rolled-back-customer", Password: "unused", AffCode: "aff-rolled-back", Status: common.UserStatusEnabled}
		if err := tx.Create(&customer).Error; err != nil {
			return err
		}
		_, bindErr := BindResellerCustomerFromInvitationWithTx(
			tx,
			"i1.invalid.invalid",
			customer.Id,
			ResellerRegistrationSourceReseller,
			1_700_000_000,
		)
		return bindErr
	})
	assert.ErrorIs(t, err, ErrResellerInvitationInvalid)
	var count int64
	require.NoError(t, db.Model(&User{}).Where("username = ?", "rolled-back-customer").Count(&count).Error)
	assert.Zero(t, count)
}
