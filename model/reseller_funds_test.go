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

func setupResellerFundsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousSecret := common.SessionSecret
	previousType := common.MainDatabaseType()
	common.SessionSecret = "reseller-funds-test-secret"
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&User{}, &ResellerProfile{}, &ResellerSecurity{}, &ResellerTransferPreview{},
		&ResellerIdempotencyRecord{}, &ResellerOutboundEvent{}, &ResellerQuotaTransfer{},
		&ResellerVoucherBatch{}, &ResellerVoucher{}, &ResellerLedgerTransaction{}, &ResellerLedgerLine{},
	))
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		common.SessionSecret = previousSecret
		common.SetMainDatabaseType(previousType)
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func createResellerFundsUser(t *testing.T, db *gorm.DB, username string, quota int) (User, ResellerProfile) {
	t.Helper()
	user := User{Username: username, Password: "unused", AffCode: "aff-" + username, Status: common.UserStatusEnabled, Quota: quota}
	require.NoError(t, db.Create(&user).Error)
	profile := ResellerProfile{UserId: user.Id, ReceivePublicId: fmt.Sprintf("receive-%024d", user.Id)}
	require.NoError(t, db.Create(&profile).Error)
	return user, profile
}

func TestResellerQuotaPasswordLifecycleAndResetFreeze(t *testing.T) {
	db := setupResellerFundsTestDB(t)
	user, _ := createResellerFundsUser(t, db, "security-user", 0)
	_, err := SetResellerQuotaPassword(user.Id, "12345", 100)
	assert.ErrorIs(t, err, ErrResellerQuotaPasswordInvalid)
	_, err = SetResellerQuotaPassword(user.Id, "123456", 100)
	require.NoError(t, err)
	assert.NoError(t, VerifyResellerQuotaPassword(user.Id, "123456", 101, true))
	require.NoError(t, ChangeResellerQuotaPassword(user.Id, "123456", "654321", 200))
	assert.ErrorIs(t, VerifyResellerQuotaPassword(user.Id, "123456", 201, false), ErrResellerQuotaPasswordInvalid)
	require.NoError(t, ResetResellerQuotaPassword(user.Id, "111111", 300))
	assert.ErrorIs(t, VerifyResellerQuotaPassword(user.Id, "111111", 301, true), ErrResellerOutboundFrozen)
	assert.NoError(t, VerifyResellerQuotaPassword(user.Id, "111111", 300+ResellerOutboundFreezeSeconds, true))
}

func TestResellerQuotaTransferPreviewCommitAndIdempotency(t *testing.T) {
	db := setupResellerFundsTestDB(t)
	sender, _ := createResellerFundsUser(t, db, "transfer-sender", 2_000_000)
	receiver, receiverProfile := createResellerFundsUser(t, db, "transfer-receiver", 100)
	_, err := SetResellerQuotaPassword(sender.Id, "123456", 100)
	require.NoError(t, err)
	nonce, preview, err := CreateResellerTransferPreview(sender.Id, receiverProfile.ReceivePublicId, 2, 200)
	require.NoError(t, err)
	assert.NotContains(t, preview.NonceHash, nonce)

	transfer, err := CommitResellerQuotaTransfer(sender.Id, nonce, "123456", "transfer-key", 201)
	require.NoError(t, err)
	replayed, err := CommitResellerQuotaTransfer(sender.Id, nonce, "123456", "transfer-key", 202)
	require.NoError(t, err)
	assert.Equal(t, transfer.PublicId, replayed.PublicId)

	require.NoError(t, db.First(&sender, sender.Id).Error)
	require.NoError(t, db.First(&receiver, receiver.Id).Error)
	assert.Equal(t, 1_000_000, sender.Quota)
	assert.Equal(t, 1_000_100, receiver.Quota)
	var transferCount, journalCount int64
	require.NoError(t, db.Model(&ResellerQuotaTransfer{}).Count(&transferCount).Error)
	require.NoError(t, db.Model(&ResellerLedgerTransaction{}).Count(&journalCount).Error)
	assert.EqualValues(t, 1, transferCount)
	assert.EqualValues(t, 1, journalCount)
}

func TestConvertResellerCommissionIsIdempotent(t *testing.T) {
	db := setupResellerFundsTestDB(t)
	user, profile := createResellerFundsUser(t, db, "convert-user", 50)
	require.NoError(t, db.Model(&profile).Update("available_commission_quota", 1_000_000).Error)
	_, err := SetResellerQuotaPassword(user.Id, "123456", 100)
	require.NoError(t, err)
	first, err := ConvertResellerCommission(user.Id, 1, "123456", "convert-key", 200)
	require.NoError(t, err)
	second, err := ConvertResellerCommission(user.Id, 1, "123456", "convert-key", 201)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	require.NoError(t, db.First(&user, user.Id).Error)
	require.NoError(t, db.First(&profile, profile.Id).Error)
	assert.Equal(t, 500_050, user.Quota)
	assert.EqualValues(t, 500_000, profile.AvailableCommissionQuota)
}

func TestVoucherEscrowRevealRedeemAndSharedRollingLimit(t *testing.T) {
	db := setupResellerFundsTestDB(t)
	issuer, receiverProfile := createResellerFundsUser(t, db, "voucher-issuer", 2_100_000_000)
	redeemer, _ := createResellerFundsUser(t, db, "voucher-redeemer", 0)
	_, err := SetResellerQuotaPassword(issuer.Id, "123456", 100)
	require.NoError(t, err)

	// Transfer 2000 and issue two 1000 vouchers: both operation classes share the 4000 window.
	nonce, _, err := CreateResellerTransferPreview(issuer.Id, receiverProfile.ReceivePublicId, 2000, 200)
	// Self destination is rejected; use redeemer profile instead.
	assert.Error(t, err)
	var redeemerProfile ResellerProfile
	require.NoError(t, db.Where("user_id = ?", redeemer.Id).First(&redeemerProfile).Error)
	nonce, _, err = CreateResellerTransferPreview(issuer.Id, redeemerProfile.ReceivePublicId, 2000, 200)
	require.NoError(t, err)
	_, err = CommitResellerQuotaTransfer(issuer.Id, nonce, "123456", "limit-transfer", 201)
	require.NoError(t, err)

	batch, codes, err := IssueResellerVoucherBatch(issuer.Id, 2, 1000, "campaign", "123456", "voucher-key", 202)
	require.NoError(t, err)
	require.Len(t, codes, 2)
	revealed, err := RevealResellerVoucherBatch(issuer.Id, batch.PublicId, "123456", 203)
	require.NoError(t, err)
	assert.Equal(t, codes, revealed)

	_, _, err = IssueResellerVoucherBatch(issuer.Id, 1, 1, "over-limit", "123456", "voucher-over", 204)
	assert.ErrorIs(t, err, ErrResellerRollingLimit)
	quota, err := RedeemResellerVoucher(codes[0], redeemer.Id, 205)
	require.NoError(t, err)
	assert.Equal(t, 500_000_000, quota)
	_, err = RedeemResellerVoucher(codes[0], redeemer.Id, 206)
	assert.ErrorIs(t, err, ErrResellerVoucherInvalid)

	var stored ResellerVoucher
	require.NoError(t, db.Where("batch_id = ?", batch.Id).First(&stored).Error)
	assert.NotContains(t, stored.CodeCiphertext, codes[0])
	assert.NotEqual(t, codes[0], stored.CodeDigest)
}
