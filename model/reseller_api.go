package model

import (
	"errors"

	"gorm.io/gorm"
)

var (
	ErrResellerNotEnabled = errors.New("reseller is not enabled")
	ErrResellerForbidden  = errors.New("reseller resource is forbidden")
)

type ResellerCustomerListItem struct {
	BindingId          int64  `json:"binding_id"`
	CustomerId         int    `json:"customer_id"`
	Username           string `json:"username"`
	DisplayName        string `json:"display_name"`
	Group              string `json:"group"`
	Quota              int    `json:"quota"`
	UsedQuota          int    `json:"used_quota"`
	RegistrationSource string `json:"registration_source"`
	BoundAt            int64  `json:"bound_at"`
	PricingVersion     int64  `json:"pricing_version"`
}

type ResellerTransferListItem struct {
	PublicId           string `json:"public_id"`
	Direction          string `json:"direction"`
	CounterpartyUserId int    `json:"counterparty_user_id"`
	CounterpartyName   string `json:"counterparty_name"`
	Amount             int    `json:"amount"`
	Quota              int    `json:"quota"`
	CreatedAt          int64  `json:"created_at"`
}

type ResellerLedgerListItem struct {
	Id                  int64  `json:"id"`
	Reference           string `json:"reference"`
	Kind                string `json:"kind"`
	RelatedCommissionId int64  `json:"related_commission_id"`
	DeltaQuota          int64  `json:"delta_quota"`
	AmountQuota         int64  `json:"amount_quota"`
	CreatedAt           int64  `json:"created_at"`
}

type ResellerStatusSummary struct {
	Enabled                  bool   `json:"enabled"`
	Status                   string `json:"status,omitempty"`
	ReceivePublicId          string `json:"receive_public_id,omitempty"`
	PricingVersion           int64  `json:"pricing_version,omitempty"`
	PendingCommissionQuota   int64  `json:"pending_commission_quota"`
	AvailableCommissionQuota int64  `json:"available_commission_quota"`
	CustomerCount            int64  `json:"customer_count"`
	WalletQuota              int    `json:"wallet_quota"`
	OutboundUsed24h          int64  `json:"outbound_used_24h"`
	CreatedAt                int64  `json:"created_at,omitempty"`
}

func GetResellerProfile(userId int) (*ResellerProfile, error) {
	var profile ResellerProfile
	if err := DB.Where("user_id = ?", userId).First(&profile).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrResellerNotEnabled
		}
		return nil, err
	}
	return &profile, nil
}

func CreateResellerProfile(userId int, now int64) (*ResellerProfile, error) {
	if userId <= 0 {
		return nil, ErrResellerNotEnabled
	}
	if existing, err := GetResellerProfile(userId); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrResellerNotEnabled) {
		return nil, err
	}

	for range 3 {
		receiveId, err := resellerPublicId("rr_")
		if err != nil {
			return nil, err
		}
		profile := ResellerProfile{
			UserId: userId, Status: ResellerStatusActive, ReceivePublicId: receiveId,
			PricingVersion: 1, CreatedAt: resellerNow(now), UpdatedAt: resellerNow(now),
		}
		if err := DB.Create(&profile).Error; err == nil {
			return &profile, nil
		}
		if existing, findErr := GetResellerProfile(userId); findErr == nil {
			return existing, nil
		}
	}
	return nil, errors.New("failed to allocate reseller receive address")
}

func GetResellerStatusSummary(userId int, now int64) (*ResellerStatusSummary, error) {
	profile, err := GetResellerProfile(userId)
	if errors.Is(err, ErrResellerNotEnabled) {
		quota, quotaErr := GetUserQuota(userId, true)
		if quotaErr != nil {
			return nil, quotaErr
		}
		return &ResellerStatusSummary{Enabled: false, WalletQuota: quota}, nil
	}
	if err != nil {
		return nil, err
	}
	var customerCount int64
	if err := DB.Model(&ResellerCustomer{}).Where("reseller_id = ?", userId).Count(&customerCount).Error; err != nil {
		return nil, err
	}
	var outboundUsed int64
	if err := DB.Model(&ResellerOutboundEvent{}).Where("user_id = ? AND created_at > ?", userId, resellerNow(now)-24*60*60).
		Select("COALESCE(SUM(amount), 0)").Scan(&outboundUsed).Error; err != nil {
		return nil, err
	}
	quota, err := GetUserQuota(userId, true)
	if err != nil {
		return nil, err
	}
	return &ResellerStatusSummary{
		Enabled: true, Status: profile.Status, ReceivePublicId: profile.ReceivePublicId,
		PricingVersion: profile.PricingVersion, PendingCommissionQuota: profile.PendingCommissionQuota,
		AvailableCommissionQuota: profile.AvailableCommissionQuota, CustomerCount: customerCount,
		WalletQuota: quota, OutboundUsed24h: outboundUsed, CreatedAt: profile.CreatedAt,
	}, nil
}

func GetResellerOwnedCustomer(resellerId int, bindingId int64) (*ResellerCustomer, error) {
	var binding ResellerCustomer
	if err := DB.Where("id = ? AND reseller_id = ?", bindingId, resellerId).First(&binding).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrResellerForbidden
		}
		return nil, err
	}
	return &binding, nil
}

func ListResellerCustomers(resellerId int, offset int, limit int) ([]ResellerCustomerListItem, int64, error) {
	query := DB.Model(&ResellerCustomer{}).Where("reseller_id = ?", resellerId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var bindings []ResellerCustomer
	if err := query.Order("bound_at DESC, id DESC").Offset(offset).Limit(limit).Find(&bindings).Error; err != nil {
		return nil, 0, err
	}
	items := make([]ResellerCustomerListItem, 0, len(bindings))
	for _, binding := range bindings {
		var user User
		if err := DB.Select("id", "username", "display_name", "group", "quota", "used_quota").First(&user, binding.CustomerId).Error; err != nil {
			return nil, 0, err
		}
		items = append(items, ResellerCustomerListItem{
			BindingId: binding.Id, CustomerId: binding.CustomerId, Username: user.Username,
			DisplayName: user.DisplayName, Group: user.Group, Quota: user.Quota, UsedQuota: user.UsedQuota,
			RegistrationSource: binding.RegistrationSource, BoundAt: binding.BoundAt, PricingVersion: binding.PricingVersion,
		})
	}
	return items, total, nil
}

func ListResellerTransfers(userId int, offset int, limit int) ([]ResellerTransferListItem, int64, error) {
	query := DB.Model(&ResellerQuotaTransfer{}).Where("sender_id = ? OR receiver_id = ?", userId, userId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ResellerQuotaTransfer
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items := make([]ResellerTransferListItem, 0, len(rows))
	for _, row := range rows {
		direction, counterpartyId := "received", row.SenderId
		if row.SenderId == userId {
			direction, counterpartyId = "sent", row.ReceiverId
		}
		name, _ := GetUsernameById(counterpartyId, true)
		items = append(items, ResellerTransferListItem{row.PublicId, direction, counterpartyId, name, row.Amount, row.Quota, row.CreatedAt})
	}
	return items, total, nil
}

func ListResellerLedger(userId int, offset int, limit int) ([]ResellerLedgerListItem, int64, error) {
	base := DB.Table("reseller_ledger_transactions rt").
		Joins("JOIN reseller_ledger_lines rl ON rl.transaction_id = rt.id").
		Where("rl.owner_user_id = ?", userId)
	var total int64
	if err := base.Distinct("rt.id").Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]ResellerLedgerListItem, 0)
	err := base.Select("rt.id, rt.reference, rt.kind, rt.related_commission_id, SUM(rl.delta_quota) AS delta_quota, MAX(ABS(rl.delta_quota)) AS amount_quota, rt.created_at").
		Group("rt.id, rt.reference, rt.kind, rt.related_commission_id, rt.created_at").
		Order("rt.id DESC").Offset(offset).Limit(limit).Scan(&items).Error
	return items, total, err
}

func ListResellerVoucherBatches(userId int, offset int, limit int) ([]ResellerVoucherBatch, int64, error) {
	query := DB.Model(&ResellerVoucherBatch{}).Where("issuer_id = ?", userId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]ResellerVoucherBatch, 0)
	err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

func ListResellerVouchers(userId int, offset int, limit int) ([]ResellerVoucher, int64, error) {
	query := DB.Model(&ResellerVoucher{}).Where("issuer_id = ?", userId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]ResellerVoucher, 0)
	err := query.Select("id", "public_id", "batch_id", "issuer_id", "amount", "quota", "redeemed_by", "redeemed_at", "created_at").
		Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

func GetResellerSecurityStatus(userId int, now int64) (*ResellerSecurity, bool, error) {
	var security ResellerSecurity
	if err := DB.Select("id", "user_id", "outbound_frozen_until", "password_version", "password_updated_at", "created_at", "updated_at").
		Where("user_id = ?", userId).First(&security).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &ResellerSecurity{UserId: userId}, false, nil
		}
		return nil, false, err
	}
	return &security, security.OutboundFrozenUntil > resellerNow(now), nil
}

func RotateResellerReceiveAddress(userId int) (string, error) {
	for range 3 {
		receiveId, err := resellerPublicId("rr_")
		if err != nil {
			return "", err
		}
		result := DB.Model(&ResellerProfile{}).Where("user_id = ? AND status = ?", userId, ResellerStatusActive).
			Update("receive_public_id", receiveId)
		if result.Error == nil && result.RowsAffected == 1 {
			return receiveId, nil
		}
		if result.Error == nil {
			return "", ErrResellerNotEnabled
		}
	}
	return "", errors.New("failed to rotate reseller receive address")
}
