package model

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrResellerCommissionInvalid           = errors.New("reseller commission is invalid")
	ErrResellerCommissionReferenceConflict = errors.New("reseller commission reference conflict")
)

type ResellerActivePricing struct {
	ResellerId        int
	CustomerId        int
	CustomerBindingId int64
	MultiplierBps     int
	MultiplierSource  ResellerMultiplierSource
}

type CreateResellerCommissionParams struct {
	RequestReference  string
	ResellerId        int
	CustomerId        int
	CustomerBindingId int64
	MultiplierBps     int
	MultiplierSource  string
	BaseQuota         int
	RetailQuota       int
	Now               time.Time
}

func ResolveActiveResellerPricing(customerId int, groupName string, now int64) (*ResellerActivePricing, error) {
	if customerId <= 0 || DB == nil {
		return nil, nil
	}

	var binding ResellerCustomer
	if err := DB.Where("customer_id = ?", customerId).First(&binding).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	var profile ResellerProfile
	if err := DB.Where("user_id = ? AND status = ?", binding.ResellerId, ResellerStatusActive).First(&profile).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	defaultRules, err := GetResellerPricingRules(ResellerPricingOwnerDefault, profile.Id)
	if err != nil {
		return nil, err
	}
	customerRules, err := GetResellerPricingRules(ResellerPricingOwnerCustomer, binding.Id)
	if err != nil {
		return nil, err
	}
	resolved := ResolveResellerMultiplier(defaultRules, customerRules, groupName, now)
	return &ResellerActivePricing{
		ResellerId:        binding.ResellerId,
		CustomerId:        binding.CustomerId,
		CustomerBindingId: binding.Id,
		MultiplierBps:     resolved.MultiplierBps,
		MultiplierSource:  resolved.Source,
	}, nil
}

func NextResellerCommissionReleaseAt(now time.Time) int64 {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	localNow := now.In(location)
	release := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 4, 10, 0, 0, location)
	if !release.After(localNow) {
		release = release.AddDate(0, 0, 1)
	}
	return release.Unix()
}

func validateResellerCommissionParams(params CreateResellerCommissionParams) error {
	if params.RequestReference == "" || len(params.RequestReference) > 191 ||
		params.ResellerId <= 0 || params.CustomerId <= 0 || params.CustomerBindingId <= 0 ||
		params.BaseQuota < 0 || params.RetailQuota < params.BaseQuota ||
		params.MultiplierBps < ResellerMultiplierBaseBps {
		return ErrResellerCommissionInvalid
	}
	return nil
}

// CreateResellerCommission is replay-safe. The first successful insert is
// authoritative; a repeated reference must describe the exact same quote.
func CreateResellerCommission(params CreateResellerCommissionParams) (*ResellerCommissionEntry, error) {
	if err := validateResellerCommissionParams(params); err != nil {
		return nil, err
	}
	commissionQuota := params.RetailQuota - params.BaseQuota
	if commissionQuota <= 0 {
		return nil, nil
	}
	if params.Now.IsZero() {
		params.Now = time.Now()
	}

	entry := ResellerCommissionEntry{
		RequestReference:  params.RequestReference,
		ResellerId:        params.ResellerId,
		CustomerId:        params.CustomerId,
		CustomerBindingId: params.CustomerBindingId,
		MultiplierBps:     params.MultiplierBps,
		MultiplierSource:  params.MultiplierSource,
		BaseQuota:         params.BaseQuota,
		RetailQuota:       params.RetailQuota,
		CommissionQuota:   commissionQuota,
		Status:            ResellerCommissionStatusPending,
		ReleaseAt:         NextResellerCommissionReleaseAt(params.Now),
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
		if result.Error != nil {
			return result.Error
		}
		var persisted ResellerCommissionEntry
		if err := tx.Where("request_reference = ?", params.RequestReference).First(&persisted).Error; err != nil {
			return err
		}
		if persisted.ResellerId != entry.ResellerId ||
			persisted.CustomerId != entry.CustomerId ||
			persisted.CustomerBindingId != entry.CustomerBindingId ||
			persisted.MultiplierBps != entry.MultiplierBps ||
			persisted.MultiplierSource != entry.MultiplierSource ||
			persisted.BaseQuota != entry.BaseQuota ||
			persisted.RetailQuota != entry.RetailQuota ||
			persisted.CommissionQuota != entry.CommissionQuota {
			return fmt.Errorf("%w: %s", ErrResellerCommissionReferenceConflict, params.RequestReference)
		}
		entry = persisted
		return postCommissionAccrualWithTx(tx, &entry)
	})
	if err != nil {
		return nil, err
	}
	return &entry, nil
}
