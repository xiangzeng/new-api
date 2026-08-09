package model

const (
	ResellerStatusActive = "active"
	ResellerStatusFrozen = "frozen"

	ResellerRegistrationSourcePrimary       = "primary"
	ResellerRegistrationSourceReseller      = "reseller"
	ResellerRegistrationSourceAdmin         = "admin"
	ResellerRegistrationSourceLegacyUnknown = "legacy_unknown"

	ResellerPricingOwnerDefault  = "default"
	ResellerPricingOwnerCustomer = "customer"

	ResellerCommissionStatusPending   = "pending"
	ResellerCommissionStatusAvailable = "available"

	ResellerLedgerKindCommissionAccrual = "commission_accrual"
	ResellerLedgerKindCommissionRelease = "commission_release"
	ResellerLedgerKindCommissionConvert = "commission_convert"
	ResellerLedgerKindQuotaTransfer     = "quota_transfer"
	ResellerLedgerKindVoucherEscrow     = "voucher_escrow"
	ResellerLedgerKindVoucherRedeem     = "voucher_redeem"

	ResellerLedgerAccountPlatformCommissionExpense = "platform_commission_expense"
	ResellerLedgerAccountCommissionPending         = "commission_pending"
	ResellerLedgerAccountCommissionAvailable       = "commission_available"
	ResellerLedgerAccountAPIWallet                 = "api_wallet"
	ResellerLedgerAccountVoucherEscrow             = "voucher_escrow"
)

// ResellerProfile is the reseller-level aggregate. Commission balances are
// materialized projections backed by the reseller ledger added in a later phase.
type ResellerProfile struct {
	Id                       int64  `json:"id" gorm:"primaryKey"`
	UserId                   int    `json:"user_id" gorm:"not null;uniqueIndex:ux_reseller_profiles_user"`
	Status                   string `json:"status" gorm:"type:varchar(16);not null;default:'active';index"`
	PendingCommissionQuota   int64  `json:"pending_commission_quota" gorm:"type:bigint;not null;default:0"`
	AvailableCommissionQuota int64  `json:"available_commission_quota" gorm:"type:bigint;not null;default:0"`
	ReceivePublicId          string `json:"receive_public_id" gorm:"type:varchar(32);not null;uniqueIndex:ux_reseller_profiles_receive"`
	PricingVersion           int64  `json:"pricing_version" gorm:"type:bigint;not null;default:1"`
	CreatedAt                int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt                int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// ResellerCustomer is the immutable, one-level ownership edge between a
// reseller and a customer. CustomerId is globally unique by design.
// Note is a private label the owning reseller writes for itself; the customer
// never sees it and it never replaces the account username.
type ResellerCustomer struct {
	Id                 int64  `json:"id" gorm:"primaryKey"`
	ResellerId         int    `json:"reseller_id" gorm:"not null;index:idx_reseller_customers_reseller_bound,priority:1"`
	CustomerId         int    `json:"customer_id" gorm:"not null;uniqueIndex:ux_reseller_customers_customer"`
	RegistrationSource string `json:"registration_source" gorm:"type:varchar(24);not null;default:'legacy_unknown';index"`
	Note               string `json:"note" gorm:"type:varchar(255);not null;default:''"`
	BoundAt            int64  `json:"bound_at" gorm:"type:bigint;not null;index:idx_reseller_customers_reseller_bound,priority:2"`
	PricingVersion     int64  `json:"pricing_version" gorm:"type:bigint;not null;default:1"`
	CreatedAt          int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// ResellerPricingRule stores a relative multiplier. GroupName is empty for an
// overall rule so the composite unique index behaves consistently on all three
// supported databases without nullable-index differences.
type ResellerPricingRule struct {
	Id                   int64  `json:"id" gorm:"primaryKey"`
	OwnerType            string `json:"owner_type" gorm:"type:varchar(16);not null;uniqueIndex:ux_reseller_pricing_scope,priority:1"`
	OwnerId              int64  `json:"owner_id" gorm:"not null;uniqueIndex:ux_reseller_pricing_scope,priority:2"`
	GroupName            string `json:"group_name" gorm:"type:varchar(64);not null;default:'';uniqueIndex:ux_reseller_pricing_scope,priority:3"`
	CurrentMultiplierBps int    `json:"current_multiplier_bps" gorm:"not null;default:10000"`
	PendingMultiplierBps int    `json:"pending_multiplier_bps" gorm:"not null;default:0"`
	PendingEffectiveAt   int64  `json:"pending_effective_at" gorm:"type:bigint;not null;default:0;index"`
	Version              int64  `json:"version" gorm:"type:bigint;not null;default:1"`
	CreatedAt            int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt            int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// ResellerCommissionEntry is the request-level accounting source for reseller
// earnings. A stable request reference makes settlement callbacks replay-safe.
type ResellerCommissionEntry struct {
	Id                int64  `json:"id" gorm:"primaryKey"`
	RequestReference  string `json:"request_reference" gorm:"type:varchar(191);not null;uniqueIndex:ux_reseller_commission_reference"`
	ResellerId        int    `json:"reseller_id" gorm:"not null;index:idx_reseller_commission_owner_status_release,priority:1"`
	CustomerId        int    `json:"customer_id" gorm:"not null;index"`
	CustomerBindingId int64  `json:"customer_binding_id" gorm:"not null;index"`
	MultiplierBps     int    `json:"multiplier_bps" gorm:"not null"`
	MultiplierSource  string `json:"multiplier_source" gorm:"type:varchar(32);not null"`
	BaseQuota         int    `json:"base_quota" gorm:"not null"`
	RetailQuota       int    `json:"retail_quota" gorm:"not null"`
	CommissionQuota   int    `json:"commission_quota" gorm:"not null"`
	Status            string `json:"status" gorm:"type:varchar(16);not null;default:'pending';index:idx_reseller_commission_owner_status_release,priority:2"`
	ReleaseAt         int64  `json:"release_at" gorm:"type:bigint;not null;index:idx_reseller_commission_owner_status_release,priority:3"`
	CreatedAt         int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt         int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type ResellerLedgerTransaction struct {
	Id                  int64  `json:"id" gorm:"primaryKey"`
	Reference           string `json:"reference" gorm:"type:varchar(191);not null;uniqueIndex:ux_reseller_ledger_transaction_reference"`
	Kind                string `json:"kind" gorm:"type:varchar(32);not null;index"`
	ResellerId          int    `json:"reseller_id" gorm:"not null;index:idx_reseller_ledger_owner_created,priority:1"`
	RelatedCommissionId int64  `json:"related_commission_id" gorm:"not null;default:0;index"`
	CreatedAt           int64  `json:"created_at" gorm:"autoCreateTime;index:idx_reseller_ledger_owner_created,priority:2"`
}

type ResellerLedgerLine struct {
	Id            int64  `json:"id" gorm:"primaryKey"`
	TransactionId int64  `json:"transaction_id" gorm:"not null;uniqueIndex:ux_reseller_ledger_line_number,priority:1;index"`
	LineNumber    int    `json:"line_number" gorm:"not null;uniqueIndex:ux_reseller_ledger_line_number,priority:2"`
	Account       string `json:"account" gorm:"type:varchar(40);not null;index"`
	OwnerUserId   int    `json:"owner_user_id" gorm:"not null;default:0;index"`
	DeltaQuota    int64  `json:"delta_quota" gorm:"type:bigint;not null"`
	BalanceAfter  int64  `json:"balance_after" gorm:"type:bigint;not null;default:0"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
}

type ResellerSecurity struct {
	Id                  int64  `json:"id" gorm:"primaryKey"`
	UserId              int    `json:"user_id" gorm:"not null;uniqueIndex"`
	QuotaPasswordHash   string `json:"-" gorm:"type:varchar(255);not null"`
	OutboundFrozenUntil int64  `json:"outbound_frozen_until" gorm:"type:bigint;not null;default:0"`
	PasswordVersion     int64  `json:"password_version" gorm:"type:bigint;not null;default:1"`
	PasswordUpdatedAt   int64  `json:"password_updated_at" gorm:"type:bigint;not null"`
	CreatedAt           int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt           int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type ResellerTransferPreview struct {
	Id         int64  `json:"id" gorm:"primaryKey"`
	NonceHash  string `json:"-" gorm:"type:char(64);not null;uniqueIndex"`
	SenderId   int    `json:"sender_id" gorm:"not null;index"`
	ReceiverId int    `json:"receiver_id" gorm:"not null"`
	Amount     int    `json:"amount" gorm:"not null"`
	Quota      int    `json:"quota" gorm:"not null"`
	ExpiresAt  int64  `json:"expires_at" gorm:"type:bigint;not null;index"`
	ConsumedAt int64  `json:"consumed_at" gorm:"type:bigint;not null;default:0"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime"`
}

type ResellerIdempotencyRecord struct {
	Id          int64  `json:"id" gorm:"primaryKey"`
	UserId      int    `json:"user_id" gorm:"not null;uniqueIndex:ux_reseller_idempotency_scope,priority:1"`
	Operation   string `json:"operation" gorm:"type:varchar(32);not null;uniqueIndex:ux_reseller_idempotency_scope,priority:2"`
	Key         string `json:"key" gorm:"type:varchar(128);not null;uniqueIndex:ux_reseller_idempotency_scope,priority:3"`
	RequestHash string `json:"request_hash" gorm:"type:char(64);not null"`
	ResultRef   string `json:"result_ref" gorm:"type:varchar(191);not null"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime"`
}

type ResellerOutboundEvent struct {
	Id        int64  `json:"id" gorm:"primaryKey"`
	UserId    int    `json:"user_id" gorm:"not null;index:idx_reseller_outbound_window,priority:1"`
	Kind      string `json:"kind" gorm:"type:varchar(24);not null"`
	Amount    int    `json:"amount" gorm:"not null"`
	Reference string `json:"reference" gorm:"type:varchar(191);not null;uniqueIndex"`
	CreatedAt int64  `json:"created_at" gorm:"type:bigint;not null;index:idx_reseller_outbound_window,priority:2"`
}

type ResellerQuotaTransfer struct {
	Id         int64  `json:"id" gorm:"primaryKey"`
	PublicId   string `json:"public_id" gorm:"type:varchar(32);not null;uniqueIndex"`
	SenderId   int    `json:"sender_id" gorm:"not null;index"`
	ReceiverId int    `json:"receiver_id" gorm:"not null;index"`
	Amount     int    `json:"amount" gorm:"not null"`
	Quota      int    `json:"quota" gorm:"not null"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime"`
}

type ResellerVoucherBatch struct {
	Id         int64  `json:"id" gorm:"primaryKey"`
	PublicId   string `json:"public_id" gorm:"type:varchar(32);not null;uniqueIndex"`
	IssuerId   int    `json:"issuer_id" gorm:"not null;index"`
	Count      int    `json:"count" gorm:"not null"`
	Amount     int    `json:"amount" gorm:"not null"`
	TotalQuota int    `json:"total_quota" gorm:"not null"`
	Note       string `json:"note" gorm:"type:varchar(255);not null;default:''"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime"`
}

type ResellerVoucher struct {
	Id             int64  `json:"id" gorm:"primaryKey"`
	PublicId       string `json:"public_id" gorm:"type:varchar(32);not null;uniqueIndex"`
	BatchId        int64  `json:"batch_id" gorm:"not null;index"`
	IssuerId       int    `json:"issuer_id" gorm:"not null;index"`
	CodeDigest     string `json:"-" gorm:"type:char(64);not null;uniqueIndex"`
	CodeCiphertext string `json:"-" gorm:"type:text;not null"`
	Amount         int    `json:"amount" gorm:"not null"`
	Quota          int    `json:"quota" gorm:"not null"`
	RedeemedBy     int    `json:"redeemed_by" gorm:"not null;default:0"`
	RedeemedAt     int64  `json:"redeemed_at" gorm:"type:bigint;not null;default:0"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
}
