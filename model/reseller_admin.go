package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// ResellerAdminBinding is the admin console view of one customer's direct
// reseller ownership. It also reports whether that user runs a reseller center
// of its own, because the two roles are independent and the operator needs to
// tell them apart before rebinding anyone.
type ResellerAdminBinding struct {
	CustomerId           int    `json:"customer_id"`
	CustomerUsername     string `json:"customer_username"`
	Bound                bool   `json:"bound"`
	BindingId            int64  `json:"binding_id"`
	ResellerId           int    `json:"reseller_id"`
	ResellerUsername     string `json:"reseller_username"`
	ResellerStatus       string `json:"reseller_status"`
	RegistrationSource   string `json:"registration_source"`
	BoundAt              int64  `json:"bound_at"`
	CurrentMultiplierBps int    `json:"current_multiplier_bps"`
	MultiplierSource     string `json:"multiplier_source"`
	IsReseller           bool   `json:"is_reseller"`
	OwnCustomerCount     int64  `json:"own_customer_count"`
}

func AdminGetResellerBinding(customerId int, now int64) (*ResellerAdminBinding, error) {
	if customerId <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var customer User
	if err := DB.Select("id", "username", "group").First(&customer, customerId).Error; err != nil {
		return nil, err
	}
	result := &ResellerAdminBinding{CustomerId: customer.Id, CustomerUsername: customer.Username}

	var ownProfile ResellerProfile
	err := DB.Where("user_id = ?", customerId).First(&ownProfile).Error
	if err == nil {
		result.IsReseller = true
		if err := DB.Model(&ResellerCustomer{}).Where("reseller_id = ?", customerId).
			Count(&result.OwnCustomerCount).Error; err != nil {
			return nil, err
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var binding ResellerCustomer
	err = DB.Where("customer_id = ?", customerId).First(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	result.Bound = true
	result.BindingId = binding.Id
	result.ResellerId = binding.ResellerId
	result.RegistrationSource = binding.RegistrationSource
	result.BoundAt = binding.BoundAt
	if username, err := GetUsernameById(binding.ResellerId, true); err == nil {
		result.ResellerUsername = username
	}
	var resellerProfile ResellerProfile
	if err := DB.Where("user_id = ?", binding.ResellerId).First(&resellerProfile).Error; err == nil {
		result.ResellerStatus = resellerProfile.Status
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	pricing, err := ResolveActiveResellerPricing(customerId, customer.Group, resellerNow(now))
	if err != nil {
		return nil, err
	}
	if pricing != nil {
		result.CurrentMultiplierBps = pricing.MultiplierBps
		result.MultiplierSource = string(pricing.MultiplierSource)
	}
	return result, nil
}

// ensureResellerProfileWithTx opens the reseller center for an admin-selected
// reseller that never opened it itself. A collision on the random receive code
// aborts the whole binding instead of retrying, because a failed insert also
// aborts the surrounding PostgreSQL transaction.
func ensureResellerProfileWithTx(tx *gorm.DB, userId int, now int64) (*ResellerProfile, error) {
	var profile ResellerProfile
	err := lockForUpdate(tx).Where("user_id = ?", userId).First(&profile).Error
	if err == nil {
		if profile.Status != ResellerStatusActive {
			return nil, ErrResellerForbidden
		}
		return &profile, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	receiveId, err := resellerReceiveCode()
	if err != nil {
		return nil, err
	}
	profile = ResellerProfile{
		UserId: userId, Status: ResellerStatusActive, ReceivePublicId: receiveId,
		PricingVersion: 1, CreatedAt: resellerNow(now), UpdatedAt: resellerNow(now),
	}
	if err := tx.Create(&profile).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}

// AdminBindResellerCustomer attaches an existing user to a reseller outside the
// invitation flow. The reseller is addressed by id when positive and by
// username otherwise, and the ownership edge itself is written by the same
// helper the registration path uses so both channels share one set of
// invariants: one direct reseller per customer, no self binding, and a synced
// users.inviter_id.
func AdminBindResellerCustomer(resellerId int, resellerUsername string, customerId int, now int64) (*ResellerCustomer, error) {
	if customerId <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	resellerUsername = strings.TrimSpace(resellerUsername)
	if resellerId <= 0 && resellerUsername == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var binding *ResellerCustomer
	err := DB.Transaction(func(tx *gorm.DB) error {
		var reseller User
		query := tx.Select("id", "username", "status")
		if resellerId > 0 {
			query = query.Where("id = ?", resellerId)
		} else {
			query = query.Where("username = ?", resellerUsername)
		}
		if err := query.First(&reseller).Error; err != nil {
			return err
		}
		if reseller.Status != common.UserStatusEnabled {
			return ErrResellerForbidden
		}
		if reseller.Id == customerId {
			return ErrResellerSelfBinding
		}
		if _, err := ensureResellerProfileWithTx(tx, reseller.Id, now); err != nil {
			return err
		}
		created, err := createResellerCustomerBindingWithTx(
			tx, reseller.Id, customerId, ResellerRegistrationSourceAdmin, resellerNow(now),
		)
		if err != nil {
			return err
		}
		binding = created
		return nil
	})
	if err != nil {
		return nil, err
	}
	return binding, nil
}

// AdminUnbindResellerCustomer releases a customer so it can be bound again.
// Customer-level pricing rules are keyed by binding id, so they are removed
// with the edge; commission entries are settled history and stay untouched.
func AdminUnbindResellerCustomer(customerId int) error {
	if customerId <= 0 {
		return gorm.ErrRecordNotFound
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var binding ResellerCustomer
		if err := lockForUpdate(tx).Where("customer_id = ?", customerId).First(&binding).Error; err != nil {
			return err
		}
		if err := tx.Where("owner_type = ? AND owner_id = ?", ResellerPricingOwnerCustomer, binding.Id).
			Delete(&ResellerPricingRule{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&ResellerCustomer{}, binding.Id).Error; err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ? AND inviter_id = ?", customerId, binding.ResellerId).
			Update("inviter_id", 0).Error
	})
}
