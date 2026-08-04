package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	ResellerOperationMinAmount      = 1
	ResellerOperationMaxAmount      = 2000
	ResellerOutboundRollingDayLimit = 4000
	ResellerVoucherBatchMaxCount    = 50
	ResellerTransferPreviewTTL      = 5 * 60
	ResellerVoucherNoteMaxLength    = 255
)

var (
	ErrResellerAmountInvalid       = errors.New("reseller amount is invalid")
	ErrResellerRollingLimit        = errors.New("reseller rolling limit exceeded")
	ErrResellerIdempotencyConflict = errors.New("reseller idempotency conflict")
	ErrResellerPreviewInvalid      = errors.New("reseller transfer preview is invalid")
	ErrResellerQuotaInsufficient   = errors.New("reseller quota is insufficient")
	ErrResellerVoucherInvalid      = errors.New("reseller voucher is invalid")
)

func resellerAmountToQuota(amount int) (int, error) {
	if amount < ResellerOperationMinAmount || amount > ResellerOperationMaxAmount {
		return 0, ErrResellerAmountInvalid
	}
	quota := int64(amount) * int64(common.QuotaPerUnit)
	if quota <= 0 || quota > int64(^uint32(0)>>1) {
		return 0, ErrResellerAmountInvalid
	}
	return int(quota), nil
}

func resellerPublicId(prefix string) (string, error) {
	random, err := common.GenerateRandomCharsKey(20)
	if err != nil {
		return "", err
	}
	return prefix + random, nil
}

func resellerRequestHash(operation string, values ...any) string {
	return resellerSecretDigest("reseller-idempotency-v1", fmt.Sprintf("%s:%v", operation, values))
}

func getResellerIdempotencyWithTx(tx *gorm.DB, userId int, operation string, key string, requestHash string) (*ResellerIdempotencyRecord, error) {
	if key == "" || len(key) > 128 {
		return nil, ErrResellerIdempotencyConflict
	}
	var record ResellerIdempotencyRecord
	err := tx.Where("user_id = ? AND operation = ? AND key = ?", userId, operation, key).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if record.RequestHash != requestHash {
		return nil, ErrResellerIdempotencyConflict
	}
	return &record, nil
}

func getCompletedResellerIdempotency(userId int, operation string, key string, requestHash string) *ResellerIdempotencyRecord {
	var record ResellerIdempotencyRecord
	if err := DB.Where("user_id = ? AND operation = ? AND key = ? AND request_hash = ?", userId, operation, key, requestHash).First(&record).Error; err != nil {
		return nil
	}
	return &record
}

func ensureRollingOutboundLimitWithTx(tx *gorm.DB, userId int, amount int, now int64) error {
	var security ResellerSecurity
	if err := lockForUpdate(tx).Where("user_id = ?", userId).First(&security).Error; err != nil {
		return ErrResellerQuotaPasswordMissing
	}
	var used int64
	if err := tx.Model(&ResellerOutboundEvent{}).
		Where("user_id = ? AND created_at > ?", userId, now-24*60*60).
		Select("COALESCE(SUM(amount), 0)").Scan(&used).Error; err != nil {
		return err
	}
	if used+int64(amount) > ResellerOutboundRollingDayLimit {
		return ErrResellerRollingLimit
	}
	return nil
}

func CreateResellerTransferPreview(senderId int, receivePublicId string, amount int, now int64) (string, *ResellerTransferPreview, error) {
	quota, err := resellerAmountToQuota(amount)
	if err != nil {
		return "", nil, err
	}
	var receiver ResellerProfile
	if err := DB.Where("receive_public_id = ? AND status = ?", strings.TrimSpace(receivePublicId), ResellerStatusActive).First(&receiver).Error; err != nil {
		return "", nil, ErrResellerPreviewInvalid
	}
	if receiver.UserId == senderId {
		return "", nil, ErrResellerPreviewInvalid
	}
	nonce, err := resellerPublicId("rpv_")
	if err != nil {
		return "", nil, err
	}
	now = resellerNow(now)
	preview := ResellerTransferPreview{
		NonceHash: resellerSecretDigest("reseller-preview-v1", nonce), SenderId: senderId,
		ReceiverId: receiver.UserId, Amount: amount, Quota: quota, ExpiresAt: now + ResellerTransferPreviewTTL,
	}
	if err := DB.Create(&preview).Error; err != nil {
		return "", nil, err
	}
	return nonce, &preview, nil
}

func CommitResellerQuotaTransfer(senderId int, nonce string, password string, idempotencyKey string, now int64) (*ResellerQuotaTransfer, error) {
	now = resellerNow(now)
	if err := VerifyResellerQuotaPassword(senderId, password, now, true); err != nil {
		return nil, err
	}
	nonceHash := resellerSecretDigest("reseller-preview-v1", strings.TrimSpace(nonce))
	requestHash := resellerRequestHash("quota_transfer", nonceHash)
	var transfer ResellerQuotaTransfer
	err := DB.Transaction(func(tx *gorm.DB) error {
		existing, err := getResellerIdempotencyWithTx(tx, senderId, "quota_transfer", idempotencyKey, requestHash)
		if err != nil {
			return err
		}
		if existing != nil {
			return tx.Where("public_id = ?", existing.ResultRef).First(&transfer).Error
		}
		var preview ResellerTransferPreview
		if err := lockForUpdate(tx).Where("nonce_hash = ? AND sender_id = ? AND consumed_at = 0 AND expires_at >= ?", nonceHash, senderId, now).First(&preview).Error; err != nil {
			return ErrResellerPreviewInvalid
		}
		if err := ensureRollingOutboundLimitWithTx(tx, senderId, preview.Amount, now); err != nil {
			return err
		}
		publicId, err := resellerPublicId("rtx_")
		if err != nil {
			return err
		}
		transfer = ResellerQuotaTransfer{PublicId: publicId, SenderId: senderId, ReceiverId: preview.ReceiverId, Amount: preview.Amount, Quota: preview.Quota}
		if err := tx.Create(&transfer).Error; err != nil {
			return err
		}
		debit := tx.Model(&User{}).Where("id = ? AND quota >= ?", senderId, preview.Quota).Update("quota", gorm.Expr("quota - ?", preview.Quota))
		if debit.Error != nil || debit.RowsAffected != 1 {
			return ErrResellerQuotaInsufficient
		}
		if err := tx.Model(&User{}).Where("id = ?", preview.ReceiverId).Update("quota", gorm.Expr("quota + ?", preview.Quota)).Error; err != nil {
			return err
		}
		if _, _, err := createResellerLedgerTransactionWithTx(tx, publicId, ResellerLedgerKindQuotaTransfer, senderId, 0, []ResellerLedgerLineInput{
			{Account: ResellerLedgerAccountAPIWallet, OwnerUserId: senderId, DeltaQuota: -int64(preview.Quota)},
			{Account: ResellerLedgerAccountAPIWallet, OwnerUserId: preview.ReceiverId, DeltaQuota: int64(preview.Quota)},
		}); err != nil {
			return err
		}
		if err := tx.Create(&ResellerOutboundEvent{UserId: senderId, Kind: "transfer", Amount: preview.Amount, Reference: publicId, CreatedAt: now}).Error; err != nil {
			return err
		}
		if result := tx.Model(&ResellerTransferPreview{}).Where("id = ? AND consumed_at = 0", preview.Id).Update("consumed_at", now); result.Error != nil || result.RowsAffected != 1 {
			return ErrResellerPreviewInvalid
		}
		return tx.Create(&ResellerIdempotencyRecord{UserId: senderId, Operation: "quota_transfer", Key: idempotencyKey, RequestHash: requestHash, ResultRef: publicId}).Error
	})
	if err != nil {
		if completed := getCompletedResellerIdempotency(senderId, "quota_transfer", idempotencyKey, requestHash); completed != nil {
			if findErr := DB.Where("public_id = ?", completed.ResultRef).First(&transfer).Error; findErr == nil {
				return &transfer, nil
			}
		}
	}
	return &transfer, err
}

func ConvertResellerCommission(userId int, amount int, password string, idempotencyKey string, now int64) (string, error) {
	quota, err := resellerAmountToQuota(amount)
	if err != nil {
		return "", err
	}
	now = resellerNow(now)
	if err := VerifyResellerQuotaPassword(userId, password, now, false); err != nil {
		return "", err
	}
	requestHash := resellerRequestHash("commission_convert", amount)
	resultRef := ""
	err = DB.Transaction(func(tx *gorm.DB) error {
		existing, err := getResellerIdempotencyWithTx(tx, userId, "commission_convert", idempotencyKey, requestHash)
		if err != nil {
			return err
		}
		if existing != nil {
			resultRef = existing.ResultRef
			return nil
		}
		resultRef, err = resellerPublicId("rcv_")
		if err != nil {
			return err
		}
		projection := tx.Model(&ResellerProfile{}).Where("user_id = ? AND available_commission_quota >= ?", userId, quota).
			Update("available_commission_quota", gorm.Expr("available_commission_quota - ?", quota))
		if projection.Error != nil || projection.RowsAffected != 1 {
			return ErrResellerQuotaInsufficient
		}
		if err := tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota + ?", quota)).Error; err != nil {
			return err
		}
		if _, _, err := createResellerLedgerTransactionWithTx(tx, resultRef, ResellerLedgerKindCommissionConvert, userId, 0, []ResellerLedgerLineInput{
			{Account: ResellerLedgerAccountCommissionAvailable, OwnerUserId: userId, DeltaQuota: -int64(quota)},
			{Account: ResellerLedgerAccountAPIWallet, OwnerUserId: userId, DeltaQuota: int64(quota)},
		}); err != nil {
			return err
		}
		return tx.Create(&ResellerIdempotencyRecord{UserId: userId, Operation: "commission_convert", Key: idempotencyKey, RequestHash: requestHash, ResultRef: resultRef}).Error
	})
	if err != nil {
		if completed := getCompletedResellerIdempotency(userId, "commission_convert", idempotencyKey, requestHash); completed != nil {
			return completed.ResultRef, nil
		}
	}
	return resultRef, err
}

func resellerCode() (string, error) {
	raw, err := common.GenerateRandomCharsKey(24)
	if err != nil {
		return "", err
	}
	return "RV-" + strings.ToUpper(raw), nil
}

func IssueResellerVoucherBatch(issuerId int, count int, amount int, note string, password string, idempotencyKey string, now int64) (*ResellerVoucherBatch, []string, error) {
	if count < 1 || count > ResellerVoucherBatchMaxCount || len([]rune(note)) > ResellerVoucherNoteMaxLength {
		return nil, nil, ErrResellerAmountInvalid
	}
	unitQuota, err := resellerAmountToQuota(amount)
	if err != nil || int64(unitQuota)*int64(count) > int64(^uint32(0)>>1) {
		return nil, nil, ErrResellerAmountInvalid
	}
	totalAmount := amount * count
	totalQuota := unitQuota * count
	now = resellerNow(now)
	if err := VerifyResellerQuotaPassword(issuerId, password, now, true); err != nil {
		return nil, nil, err
	}
	requestHash := resellerRequestHash("voucher_issue", count, amount, note)
	var batch ResellerVoucherBatch
	var revealed []string
	err = DB.Transaction(func(tx *gorm.DB) error {
		existing, err := getResellerIdempotencyWithTx(tx, issuerId, "voucher_issue", idempotencyKey, requestHash)
		if err != nil {
			return err
		}
		if existing != nil {
			return tx.Where("public_id = ?", existing.ResultRef).First(&batch).Error
		}
		if err := ensureRollingOutboundLimitWithTx(tx, issuerId, totalAmount, now); err != nil {
			return err
		}
		publicId, err := resellerPublicId("rvb_")
		if err != nil {
			return err
		}
		batch = ResellerVoucherBatch{PublicId: publicId, IssuerId: issuerId, Count: count, Amount: amount, TotalQuota: totalQuota, Note: note}
		if err := tx.Create(&batch).Error; err != nil {
			return err
		}
		vouchers := make([]ResellerVoucher, 0, count)
		for range count {
			code, err := resellerCode()
			if err != nil {
				return err
			}
			ciphertext, err := encryptResellerSecret(code)
			if err != nil {
				return err
			}
			voucherId, err := resellerPublicId("rvc_")
			if err != nil {
				return err
			}
			vouchers = append(vouchers, ResellerVoucher{PublicId: voucherId, BatchId: batch.Id, IssuerId: issuerId, CodeDigest: resellerSecretDigest("reseller-voucher-v1", code), CodeCiphertext: ciphertext, Amount: amount, Quota: unitQuota})
			revealed = append(revealed, code)
		}
		if err := tx.Create(&vouchers).Error; err != nil {
			return err
		}
		debit := tx.Model(&User{}).Where("id = ? AND quota >= ?", issuerId, totalQuota).Update("quota", gorm.Expr("quota - ?", totalQuota))
		if debit.Error != nil || debit.RowsAffected != 1 {
			return ErrResellerQuotaInsufficient
		}
		if _, _, err := createResellerLedgerTransactionWithTx(tx, publicId, ResellerLedgerKindVoucherEscrow, issuerId, 0, []ResellerLedgerLineInput{
			{Account: ResellerLedgerAccountAPIWallet, OwnerUserId: issuerId, DeltaQuota: -int64(totalQuota)},
			{Account: ResellerLedgerAccountVoucherEscrow, OwnerUserId: issuerId, DeltaQuota: int64(totalQuota)},
		}); err != nil {
			return err
		}
		if err := tx.Create(&ResellerOutboundEvent{UserId: issuerId, Kind: "voucher", Amount: totalAmount, Reference: publicId, CreatedAt: now}).Error; err != nil {
			return err
		}
		return tx.Create(&ResellerIdempotencyRecord{UserId: issuerId, Operation: "voucher_issue", Key: idempotencyKey, RequestHash: requestHash, ResultRef: publicId}).Error
	})
	if err != nil {
		if completed := getCompletedResellerIdempotency(issuerId, "voucher_issue", idempotencyKey, requestHash); completed != nil {
			if findErr := DB.Where("public_id = ?", completed.ResultRef).First(&batch).Error; findErr == nil {
				err = nil
			}
		}
	}
	if err == nil && len(revealed) == 0 {
		revealed, err = RevealResellerVoucherBatch(issuerId, batch.PublicId, password, now)
	}
	return &batch, revealed, err
}

func RevealResellerVoucherBatch(issuerId int, batchPublicId string, password string, now int64) ([]string, error) {
	if err := VerifyResellerQuotaPassword(issuerId, password, resellerNow(now), false); err != nil {
		return nil, err
	}
	var batch ResellerVoucherBatch
	if err := DB.Where("public_id = ? AND issuer_id = ?", batchPublicId, issuerId).First(&batch).Error; err != nil {
		return nil, ErrResellerVoucherInvalid
	}
	var vouchers []ResellerVoucher
	if err := DB.Where("batch_id = ?", batch.Id).Order("id asc").Find(&vouchers).Error; err != nil {
		return nil, err
	}
	codes := make([]string, 0, len(vouchers))
	for _, voucher := range vouchers {
		code, err := decryptResellerSecret(voucher.CodeCiphertext)
		if err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, nil
}

func RedeemResellerVoucher(code string, userId int, now int64) (int, error) {
	now = resellerNow(now)
	digest := resellerSecretDigest("reseller-voucher-v1", strings.TrimSpace(code))
	var quota int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var voucher ResellerVoucher
		if err := lockForUpdate(tx).Where("code_digest = ? AND redeemed_at = 0", digest).First(&voucher).Error; err != nil {
			return ErrResellerVoucherInvalid
		}
		result := tx.Model(&ResellerVoucher{}).Where("id = ? AND redeemed_at = 0", voucher.Id).Updates(map[string]any{"redeemed_by": userId, "redeemed_at": now})
		if result.Error != nil || result.RowsAffected != 1 {
			return ErrResellerVoucherInvalid
		}
		if err := tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota + ?", voucher.Quota)).Error; err != nil {
			return err
		}
		ref := fmt.Sprintf("voucher:%d:redeem", voucher.Id)
		if _, _, err := createResellerLedgerTransactionWithTx(tx, ref, ResellerLedgerKindVoucherRedeem, voucher.IssuerId, 0, []ResellerLedgerLineInput{
			{Account: ResellerLedgerAccountVoucherEscrow, OwnerUserId: voucher.IssuerId, DeltaQuota: -int64(voucher.Quota)},
			{Account: ResellerLedgerAccountAPIWallet, OwnerUserId: userId, DeltaQuota: int64(voucher.Quota)},
		}); err != nil {
			return err
		}
		quota = voucher.Quota
		return nil
	})
	return quota, err
}
