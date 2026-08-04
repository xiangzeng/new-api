package model

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrResellerLedgerUnbalanced        = errors.New("reseller ledger transaction is unbalanced")
	ErrResellerLedgerReferenceConflict = errors.New("reseller ledger reference conflict")
	ErrResellerBalanceProjection       = errors.New("reseller balance projection mismatch")
)

type ResellerLedgerLineInput struct {
	Account     string
	OwnerUserId int
	DeltaQuota  int64
}

type ResellerCommissionReleaseBatchResult struct {
	Processed int
	Remaining int64
	LastId    int64
}

func validateResellerLedgerLines(lines []ResellerLedgerLineInput) error {
	if len(lines) < 2 {
		return ErrResellerLedgerUnbalanced
	}
	var sum int64
	for _, line := range lines {
		if line.Account == "" || line.DeltaQuota == 0 {
			return ErrResellerLedgerUnbalanced
		}
		previous := sum
		sum += line.DeltaQuota
		if (line.DeltaQuota > 0 && sum < previous) || (line.DeltaQuota < 0 && sum > previous) {
			return ErrResellerLedgerUnbalanced
		}
	}
	if sum != 0 {
		return ErrResellerLedgerUnbalanced
	}
	return nil
}

func resellerLedgerBalanceAfter(tx *gorm.DB, line ResellerLedgerLineInput) (int64, error) {
	switch line.Account {
	case ResellerLedgerAccountAPIWallet:
		var user User
		if err := tx.Select("quota").Where("id = ?", line.OwnerUserId).First(&user).Error; err != nil {
			return 0, err
		}
		return int64(user.Quota) + line.DeltaQuota, nil
	case ResellerLedgerAccountCommissionPending, ResellerLedgerAccountCommissionAvailable:
		var profile ResellerProfile
		if err := tx.Select("pending_commission_quota", "available_commission_quota").Where("user_id = ?", line.OwnerUserId).First(&profile).Error; err != nil {
			return 0, err
		}
		if line.Account == ResellerLedgerAccountCommissionPending {
			return profile.PendingCommissionQuota + line.DeltaQuota, nil
		}
		return profile.AvailableCommissionQuota + line.DeltaQuota, nil
	default:
		var balance int64
		if err := tx.Model(&ResellerLedgerLine{}).
			Where("account = ? AND owner_user_id = ?", line.Account, line.OwnerUserId).
			Select("COALESCE(SUM(delta_quota), 0)").Scan(&balance).Error; err != nil {
			return 0, err
		}
		return balance + line.DeltaQuota, nil
	}
}

// createResellerLedgerTransactionWithTx posts an immutable, balanced journal.
// The returned bool is true only for the transaction that created the header.
func createResellerLedgerTransactionWithTx(
	tx *gorm.DB,
	reference string,
	kind string,
	resellerId int,
	commissionId int64,
	lines []ResellerLedgerLineInput,
) (*ResellerLedgerTransaction, bool, error) {
	if tx == nil || reference == "" || len(reference) > 191 || kind == "" || resellerId <= 0 || commissionId < 0 {
		return nil, false, ErrResellerLedgerReferenceConflict
	}
	if err := validateResellerLedgerLines(lines); err != nil {
		return nil, false, err
	}

	journal := ResellerLedgerTransaction{
		Reference: reference, Kind: kind, ResellerId: resellerId, RelatedCommissionId: commissionId,
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&journal)
	if result.Error != nil {
		return nil, false, result.Error
	}
	created := result.RowsAffected == 1
	if !created {
		if err := tx.Where("reference = ?", reference).First(&journal).Error; err != nil {
			return nil, false, err
		}
		if journal.Kind != kind || journal.ResellerId != resellerId || journal.RelatedCommissionId != commissionId {
			return nil, false, fmt.Errorf("%w: %s", ErrResellerLedgerReferenceConflict, reference)
		}
		return &journal, false, nil
	}

	persistedLines := make([]ResellerLedgerLine, 0, len(lines))
	for index, line := range lines {
		balanceAfter, err := resellerLedgerBalanceAfter(tx, line)
		if err != nil {
			return nil, false, err
		}
		persistedLines = append(persistedLines, ResellerLedgerLine{
			TransactionId: journal.Id,
			LineNumber:    index + 1,
			Account:       line.Account,
			OwnerUserId:   line.OwnerUserId,
			DeltaQuota:    line.DeltaQuota,
			BalanceAfter:  balanceAfter,
		})
	}
	if err := tx.Create(&persistedLines).Error; err != nil {
		return nil, false, err
	}
	return &journal, true, nil
}

func postCommissionAccrualWithTx(tx *gorm.DB, entry *ResellerCommissionEntry) error {
	amount := int64(entry.CommissionQuota)
	_, created, err := createResellerLedgerTransactionWithTx(
		tx,
		fmt.Sprintf("commission:%d:accrual", entry.Id),
		ResellerLedgerKindCommissionAccrual,
		entry.ResellerId,
		entry.Id,
		[]ResellerLedgerLineInput{
			{Account: ResellerLedgerAccountPlatformCommissionExpense, OwnerUserId: 0, DeltaQuota: -amount},
			{Account: ResellerLedgerAccountCommissionPending, OwnerUserId: entry.ResellerId, DeltaQuota: amount},
		},
	)
	if err != nil || !created {
		return err
	}
	result := tx.Model(&ResellerProfile{}).
		Where("user_id = ?", entry.ResellerId).
		Update("pending_commission_quota", gorm.Expr("pending_commission_quota + ?", amount))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrResellerBalanceProjection
	}
	return nil
}

func releaseResellerCommissionWithTx(tx *gorm.DB, commissionId int64, cutoff int64) (bool, error) {
	var entry ResellerCommissionEntry
	if err := lockForUpdate(tx).
		Where("id = ? AND status = ? AND release_at <= ?", commissionId, ResellerCommissionStatusPending, cutoff).
		First(&entry).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	amount := int64(entry.CommissionQuota)
	_, created, err := createResellerLedgerTransactionWithTx(
		tx,
		fmt.Sprintf("commission:%d:release", entry.Id),
		ResellerLedgerKindCommissionRelease,
		entry.ResellerId,
		entry.Id,
		[]ResellerLedgerLineInput{
			{Account: ResellerLedgerAccountCommissionPending, OwnerUserId: entry.ResellerId, DeltaQuota: -amount},
			{Account: ResellerLedgerAccountCommissionAvailable, OwnerUserId: entry.ResellerId, DeltaQuota: amount},
		},
	)
	if err != nil {
		return false, err
	}
	if !created {
		return false, ErrResellerLedgerReferenceConflict
	}
	statusUpdate := tx.Model(&ResellerCommissionEntry{}).
		Where("id = ? AND status = ?", entry.Id, ResellerCommissionStatusPending).
		Update("status", ResellerCommissionStatusAvailable)
	if statusUpdate.Error != nil {
		return false, statusUpdate.Error
	}
	if statusUpdate.RowsAffected != 1 {
		return false, ErrResellerLedgerReferenceConflict
	}
	balanceUpdate := tx.Model(&ResellerProfile{}).
		Where("user_id = ? AND pending_commission_quota >= ?", entry.ResellerId, amount).
		Updates(map[string]any{
			"pending_commission_quota":   gorm.Expr("pending_commission_quota - ?", amount),
			"available_commission_quota": gorm.Expr("available_commission_quota + ?", amount),
		})
	if balanceUpdate.Error != nil {
		return false, balanceUpdate.Error
	}
	if balanceUpdate.RowsAffected != 1 {
		return false, ErrResellerBalanceProjection
	}
	return true, nil
}

func HasDueResellerCommissions(cutoff int64) bool {
	var count int64
	err := DB.Model(&ResellerCommissionEntry{}).
		Where("status = ? AND release_at <= ?", ResellerCommissionStatusPending, cutoff).
		Limit(1).
		Count(&count).Error
	return err == nil && count > 0
}

func ReleaseDueResellerCommissionsBatch(ctx context.Context, cutoff int64, limit int) (ResellerCommissionReleaseBatchResult, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var ids []int64
	if err := DB.WithContext(ctx).Model(&ResellerCommissionEntry{}).
		Where("status = ? AND release_at <= ?", ResellerCommissionStatusPending, cutoff).
		Order("id asc").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return ResellerCommissionReleaseBatchResult{}, err
	}

	result := ResellerCommissionReleaseBatchResult{}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		released := false
		err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var releaseErr error
			released, releaseErr = releaseResellerCommissionWithTx(tx, id, cutoff)
			return releaseErr
		})
		if err != nil {
			return result, err
		}
		if released {
			result.Processed++
			result.LastId = id
		}
	}
	if err := DB.WithContext(ctx).Model(&ResellerCommissionEntry{}).
		Where("status = ? AND release_at <= ?", ResellerCommissionStatusPending, cutoff).
		Count(&result.Remaining).Error; err != nil {
		return result, err
	}
	return result, nil
}
