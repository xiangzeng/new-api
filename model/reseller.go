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
type ResellerCustomer struct {
	Id                 int64  `json:"id" gorm:"primaryKey"`
	ResellerId         int    `json:"reseller_id" gorm:"not null;index:idx_reseller_customers_reseller_bound,priority:1"`
	CustomerId         int    `json:"customer_id" gorm:"not null;uniqueIndex:ux_reseller_customers_customer"`
	RegistrationSource string `json:"registration_source" gorm:"type:varchar(24);not null;default:'legacy_unknown';index"`
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
