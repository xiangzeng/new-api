package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var (
	ErrResellerNotEnabled  = errors.New("reseller is not enabled")
	ErrResellerForbidden   = errors.New("reseller resource is forbidden")
	ErrResellerNoteTooLong = errors.New("reseller customer note is too long")
)

type ResellerCustomerListItem struct {
	BindingId               int64  `json:"binding_id"`
	CustomerId              int    `json:"customer_id"`
	Username                string `json:"username"`
	DisplayName             string `json:"display_name"`
	Note                    string `json:"note"`
	Status                  int    `json:"status"`
	Group                   string `json:"group"`
	Quota                   int    `json:"quota"`
	UsedQuota               int    `json:"used_quota"`
	RegistrationSource      string `json:"registration_source"`
	BoundAt                 int64  `json:"bound_at"`
	PricingVersion          int64  `json:"pricing_version"`
	CurrentMultiplierBps    int    `json:"current_multiplier_bps"`
	PendingMultiplierBps    int    `json:"pending_multiplier_bps"`
	PendingEffectiveAt      int64  `json:"pending_effective_at"`
	CustomerRetailQuota     int64  `json:"customer_retail_quota"`
	ResellerRequestCount    int64  `json:"reseller_request_count"`
	ResellerCommissionQuota int64  `json:"reseller_commission_quota"`
}

type resellerCustomerTotals struct {
	CustomerId      int   `gorm:"column:customer_id"`
	RetailQuota     int64 `gorm:"column:retail_quota"`
	CommissionQuota int64 `gorm:"column:commission_quota"`
	RequestCount    int64 `gorm:"column:request_count"`
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
	Account             string `json:"account"`
	RelatedCommissionId int64  `json:"related_commission_id"`
	DeltaQuota          int64  `json:"delta_quota"`
	BalanceAfter        int64  `json:"balance_after"`
	AmountQuota         int64  `json:"amount_quota"`
	CreatedAt           int64  `json:"created_at"`
}

type ResellerStatusSummary struct {
	Enabled                  bool   `json:"enabled"`
	Status                   string `json:"status,omitempty"`
	PricingVersion           int64  `json:"pricing_version,omitempty"`
	PendingCommissionQuota   int64  `json:"pending_commission_quota"`
	AvailableCommissionQuota int64  `json:"available_commission_quota"`
	CustomerCount            int64  `json:"customer_count"`
	WalletQuota              int    `json:"wallet_quota"`
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
		receiveId, err := resellerReceiveCode()
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

func GetResellerStatusSummary(userId int, _ int64) (*ResellerStatusSummary, error) {
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
	quota, err := GetUserQuota(userId, true)
	if err != nil {
		return nil, err
	}
	return &ResellerStatusSummary{
		Enabled: true, Status: profile.Status,
		PricingVersion: profile.PricingVersion, PendingCommissionQuota: profile.PendingCommissionQuota,
		AvailableCommissionQuota: profile.AvailableCommissionQuota, CustomerCount: customerCount,
		WalletQuota: quota, CreatedAt: profile.CreatedAt,
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
	if len(bindings) == 0 {
		return make([]ResellerCustomerListItem, 0), total, nil
	}

	profile, err := GetResellerProfile(resellerId)
	if err != nil {
		return nil, 0, err
	}
	defaultRules, err := GetResellerPricingRules(ResellerPricingOwnerDefault, profile.Id)
	if err != nil {
		return nil, 0, err
	}
	customerIds := make([]int, 0, len(bindings))
	bindingIds := make([]int64, 0, len(bindings))
	for _, binding := range bindings {
		customerIds = append(customerIds, binding.CustomerId)
		bindingIds = append(bindingIds, binding.Id)
	}

	var users []User
	if err := DB.Select("id", "username", "display_name", "status", "group", "quota", "used_quota").
		Where("id IN ?", customerIds).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	usersById := make(map[int]User, len(users))
	for _, user := range users {
		usersById[user.Id] = user
	}

	var customerRuleRows []ResellerPricingRule
	if err := DB.Where("owner_type = ? AND owner_id IN ? AND group_name = ?", ResellerPricingOwnerCustomer, bindingIds, "").
		Find(&customerRuleRows).Error; err != nil {
		return nil, 0, err
	}
	customerRulesByBindingId := make(map[int64]ResellerPricingRule, len(customerRuleRows))
	for _, rule := range customerRuleRows {
		customerRulesByBindingId[rule.OwnerId] = rule
	}

	var totalRows []resellerCustomerTotals
	if err := DB.Model(&ResellerCommissionEntry{}).
		Select("customer_id, COALESCE(SUM(retail_quota), 0) AS retail_quota, COALESCE(SUM(commission_quota), 0) AS commission_quota, COUNT(*) AS request_count").
		Where("reseller_id = ? AND customer_id IN ?", resellerId, customerIds).
		Group("customer_id").Scan(&totalRows).Error; err != nil {
		return nil, 0, err
	}
	totalsByCustomerId := make(map[int]resellerCustomerTotals, len(totalRows))
	for _, row := range totalRows {
		totalsByCustomerId[row.CustomerId] = row
	}

	now := common.GetTimestamp()
	items := make([]ResellerCustomerListItem, 0, len(bindings))
	for _, binding := range bindings {
		user, ok := usersById[binding.CustomerId]
		if !ok {
			return nil, 0, gorm.ErrRecordNotFound
		}
		customerRules := make(map[string]ResellerPricingRule, 1)
		if rule, exists := customerRulesByBindingId[binding.Id]; exists {
			customerRules[""] = rule
		}
		resolved := ResolveResellerMultiplier(defaultRules, customerRules, "", now)
		pendingMultiplierBps, pendingEffectiveAt := 0, int64(0)
		if rule, exists := customerRulesByBindingId[binding.Id]; exists {
			if rule.PendingMultiplierBps > 0 && rule.PendingEffectiveAt > now {
				pendingMultiplierBps, pendingEffectiveAt = rule.PendingMultiplierBps, rule.PendingEffectiveAt
			}
		} else if rule, exists := defaultRules[""]; exists {
			if rule.PendingMultiplierBps > 0 && rule.PendingEffectiveAt > now {
				pendingMultiplierBps, pendingEffectiveAt = rule.PendingMultiplierBps, rule.PendingEffectiveAt
			}
		}
		totals := totalsByCustomerId[binding.CustomerId]
		items = append(items, ResellerCustomerListItem{
			BindingId: binding.Id, CustomerId: binding.CustomerId, Username: user.Username,
			DisplayName: user.DisplayName, Note: binding.Note, Status: user.Status, Group: user.Group,
			Quota: user.Quota, UsedQuota: user.UsedQuota,
			RegistrationSource: binding.RegistrationSource, BoundAt: binding.BoundAt, PricingVersion: binding.PricingVersion,
			CurrentMultiplierBps: resolved.MultiplierBps, PendingMultiplierBps: pendingMultiplierBps,
			PendingEffectiveAt: pendingEffectiveAt, CustomerRetailQuota: totals.RetailQuota,
			ResellerRequestCount: totals.RequestCount, ResellerCommissionQuota: totals.CommissionQuota,
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
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]ResellerLedgerListItem, 0)
	err := base.Select("rl.id, rt.reference, rt.kind, rl.account, rt.related_commission_id, rl.delta_quota, rl.balance_after, ABS(rl.delta_quota) AS amount_quota, rt.created_at").
		Order("rt.id DESC, rl.line_number ASC").Offset(offset).Limit(limit).Scan(&items).Error
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
	return ListResellerVouchersByStatus(userId, "", offset, limit)
}

func ListResellerVouchersByStatus(userId int, status string, offset int, limit int) ([]ResellerVoucher, int64, error) {
	query := DB.Model(&ResellerVoucher{}).Where("issuer_id = ?", userId)
	switch status {
	case "pending":
		query = query.Where("redeemed_at = 0")
	case "used":
		query = query.Where("redeemed_at > 0")
	}
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

// UpdateResellerCustomerNote stores the reseller-private label for one owned
// customer. It carries no funds, so ownership is the only authorization needed.
// Ownership is resolved with an explicit read because MySQL reports zero
// affected rows when an update rewrites an identical value.
func UpdateResellerCustomerNote(resellerId int, bindingId int64, note string) (string, error) {
	note = strings.TrimSpace(note)
	if len([]rune(note)) > ResellerCustomerNoteMaxLength {
		return "", ErrResellerNoteTooLong
	}
	binding, err := GetResellerOwnedCustomer(resellerId, bindingId)
	if err != nil {
		return "", err
	}
	if err := DB.Model(&ResellerCustomer{}).Where("id = ?", binding.Id).Update("note", note).Error; err != nil {
		return "", err
	}
	return note, nil
}
