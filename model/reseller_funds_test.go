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
		&User{}, &ResellerProfile{}, &ResellerCustomer{}, &ResellerSecurity{}, &ResellerTransferPreview{},
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

func bindResellerFundsCustomer(t *testing.T, db *gorm.DB, resellerId int, customerId int) ResellerCustomer {
	t.Helper()
	binding := ResellerCustomer{
		ResellerId: resellerId, CustomerId: customerId,
		RegistrationSource: ResellerRegistrationSourceReseller, BoundAt: 100, PricingVersion: 1,
	}
	require.NoError(t, db.Create(&binding).Error)
	return binding
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
	receiver, _ := createResellerFundsUser(t, db, "transfer-receiver", 100)
	binding := bindResellerFundsCustomer(t, db, sender.Id, receiver.Id)
	_, err := SetResellerQuotaPassword(sender.Id, "123456", 100)
	require.NoError(t, err)
	nonce, preview, err := CreateResellerCustomerTransferPreview(sender.Id, binding.Id, "", 1_000_000, 200)
	require.NoError(t, err)
	assert.NotContains(t, preview.NonceHash, nonce)

	transfer, err := CommitResellerQuotaTransfer(sender.Id, nonce, "123456", "transfer-key", ResellerTransferCommitExpectation{}, 201)
	require.NoError(t, err)
	replayed, err := CommitResellerQuotaTransfer(sender.Id, nonce, "123456", "transfer-key", ResellerTransferCommitExpectation{}, 202)
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
	var lines []ResellerLedgerLine
	require.NoError(t, db.Order("line_number asc").Find(&lines).Error)
	require.Len(t, lines, 2)
	assert.EqualValues(t, 1_000_000, lines[0].BalanceAfter)
	assert.EqualValues(t, 1_000_100, lines[1].BalanceAfter)
}

func TestResellerTransferOnlyReachesOwnedCustomersAndIsBoundedByWallet(t *testing.T) {
	db := setupResellerFundsTestDB(t)
	sender, _ := createResellerFundsUser(t, db, "recipient-sender", 2_000_000)
	customer, _ := createResellerFundsUser(t, db, "recipient-customer", 0)
	stranger, _ := createResellerFundsUser(t, db, "recipient-stranger", 0)
	foreign, _ := createResellerFundsUser(t, db, "recipient-foreign-reseller", 0)
	binding := bindResellerFundsCustomer(t, db, sender.Id, customer.Id)
	foreignBinding := bindResellerFundsCustomer(t, db, foreign.Id, stranger.Id)
	_, err := SetResellerQuotaPassword(sender.Id, "123456", 100)
	require.NoError(t, err)

	_, _, err = CreateResellerCustomerTransferPreview(sender.Id, 0, stranger.Username, 500_000, 200)
	assert.ErrorIs(t, err, ErrResellerRecipientNotCustomer)
	_, _, err = CreateResellerCustomerTransferPreview(sender.Id, foreignBinding.Id, "", 500_000, 200)
	assert.ErrorIs(t, err, ErrResellerRecipientNotCustomer)
	_, _, err = CreateResellerCustomerTransferPreview(sender.Id, binding.Id, "", 2_000_001, 200)
	assert.ErrorIs(t, err, ErrResellerQuotaInsufficient)

	nonce, preview, err := CreateResellerCustomerTransferPreview(sender.Id, 0, customer.Username, 500_000, 200)
	require.NoError(t, err)
	assert.Equal(t, customer.Id, preview.ReceiverId)
	_, err = CommitResellerQuotaTransfer(sender.Id, nonce, "123456", "mismatch-key", ResellerTransferCommitExpectation{
		RecipientUserId: customer.Id, RecipientName: customer.Username, Quota: 600_000,
	}, 201)
	assert.ErrorIs(t, err, ErrResellerPreviewInvalid)

	transfer, err := CommitResellerQuotaTransfer(sender.Id, nonce, "123456", "match-key", ResellerTransferCommitExpectation{
		RecipientUserId: customer.Id, RecipientName: customer.Username, Quota: 500_000,
	}, 202)
	require.NoError(t, err)
	assert.Equal(t, customer.Id, transfer.ReceiverId)
	assert.EqualValues(t, 500_000, transfer.Quota)
}

func TestResellerTransferAcceptsSubUnitQuotaBeyondVoucherAmountCap(t *testing.T) {
	db := setupResellerFundsTestDB(t)
	sender, _ := createResellerFundsUser(t, db, "fractional-sender", 3_000_000_000)
	customer, _ := createResellerFundsUser(t, db, "fractional-customer", 0)
	binding := bindResellerFundsCustomer(t, db, sender.Id, customer.Id)
	_, err := SetResellerQuotaPassword(sender.Id, "123456", 100)
	require.NoError(t, err)

	// 0.5 unit: below the whole-unit voucher minimum.
	nonce, _, err := CreateResellerCustomerTransferPreview(sender.Id, binding.Id, "", 250_000, 200)
	require.NoError(t, err)
	fractional, err := CommitResellerQuotaTransfer(sender.Id, nonce, "123456", "fractional-key", ResellerTransferCommitExpectation{}, 201)
	require.NoError(t, err)
	assert.EqualValues(t, 250_000, fractional.Quota)
	assert.Equal(t, 0, fractional.Amount)

	// 5000 units: above the 2000 per-operation voucher cap.
	nonce, _, err = CreateResellerCustomerTransferPreview(sender.Id, binding.Id, "", 2_500_000_000, 202)
	require.NoError(t, err)
	large, err := CommitResellerQuotaTransfer(sender.Id, nonce, "123456", "large-key", ResellerTransferCommitExpectation{}, 203)
	require.NoError(t, err)
	assert.EqualValues(t, 2_500_000_000, large.Quota)

	require.NoError(t, db.First(&customer, customer.Id).Error)
	assert.Equal(t, 2_500_250_000, customer.Quota)
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

func TestConvertAllResellerCommissionRequiresCurrentFullBalance(t *testing.T) {
	db := setupResellerFundsTestDB(t)
	user, profile := createResellerFundsUser(t, db, "convert-all-user", 50)
	require.NoError(t, db.Model(&profile).Update("available_commission_quota", 1_250_000).Error)
	_, err := SetResellerQuotaPassword(user.Id, "123456", 100)
	require.NoError(t, err)

	_, _, err = ConvertAllResellerCommission(user.Id, 1_000_000, "123456", "convert-all-stale", 200)
	assert.ErrorIs(t, err, ErrResellerQuotaInsufficient)
	ref, quota, err := ConvertAllResellerCommission(user.Id, 1_250_000, "123456", "convert-all-key", 201)
	require.NoError(t, err)
	assert.NotEmpty(t, ref)
	assert.EqualValues(t, 1_250_000, quota)
	replayedRef, replayedQuota, err := ConvertAllResellerCommission(user.Id, 1_250_000, "123456", "convert-all-key", 202)
	require.NoError(t, err)
	assert.Equal(t, ref, replayedRef)
	assert.Equal(t, quota, replayedQuota)
	require.NoError(t, db.First(&profile, profile.Id).Error)
	assert.Zero(t, profile.AvailableCommissionQuota)
}

func TestVoucherEscrowRevealRedeemAndVoucherOnlyRollingLimit(t *testing.T) {
	db := setupResellerFundsTestDB(t)
	issuer, _ := createResellerFundsUser(t, db, "voucher-issuer", 2_100_000_000)
	redeemer, _ := createResellerFundsUser(t, db, "voucher-redeemer", 0)
	binding := bindResellerFundsCustomer(t, db, issuer.Id, redeemer.Id)
	_, err := SetResellerQuotaPassword(issuer.Id, "123456", 100)
	require.NoError(t, err)

	// A 200-unit transfer is audited as an outbound event but must not consume
	// the rolling window that only guards user code issuance.
	nonce, _, err := CreateResellerCustomerTransferPreview(issuer.Id, binding.Id, "", 100_000_000, 200)
	require.NoError(t, err)
	_, err = CommitResellerQuotaTransfer(issuer.Id, nonce, "123456", "limit-transfer", ResellerTransferCommitExpectation{}, 201)
	require.NoError(t, err)

	batch, codes, err := IssueResellerVoucherBatch(issuer.Id, 2, 1000, "campaign", "123456", "voucher-key", 202)
	require.NoError(t, err)
	require.Len(t, codes, 2)
	revealed, err := RevealResellerVoucherBatch(issuer.Id, batch.PublicId, "123456", 203)
	require.NoError(t, err)
	assert.Equal(t, codes, revealed)

	// 2000 of the 4000 window is used, so a second 2000 batch still fits.
	_, _, err = IssueResellerVoucherBatch(issuer.Id, 2, 1000, "second", "123456", "voucher-second", 204)
	require.NoError(t, err)
	_, _, err = IssueResellerVoucherBatch(issuer.Id, 1, 1, "over-limit", "123456", "voucher-over", 205)
	assert.ErrorIs(t, err, ErrResellerRollingLimit)
	quota, err := RedeemResellerVoucher(codes[0], redeemer.Id, 206)
	require.NoError(t, err)
	assert.Equal(t, 500_000_000, quota)
	_, err = RedeemResellerVoucher(codes[0], redeemer.Id, 207)
	assert.ErrorIs(t, err, ErrResellerVoucherInvalid)

	var stored ResellerVoucher
	require.NoError(t, db.Where("batch_id = ?", batch.Id).First(&stored).Error)
	assert.NotContains(t, stored.CodeCiphertext, codes[0])
	assert.NotEqual(t, codes[0], stored.CodeDigest)
}
