package model

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const (
	ResellerMultiplierBaseBps = 10000
	ResellerMultiplierMaxBps  = 100000
	ResellerPriceIncreaseWait = int64(24 * 60 * 60)
)

var (
	ErrResellerMultiplierOutOfRange   = errors.New("reseller multiplier is out of range")
	ErrResellerPricingOwnerInvalid    = errors.New("reseller pricing owner is invalid")
	ErrResellerPricingVersionConflict = errors.New("reseller pricing version conflict")
)

type ResellerMultiplierSource string

const (
	ResellerMultiplierSourceBase            ResellerMultiplierSource = "platform_base"
	ResellerMultiplierSourceDefaultOverall  ResellerMultiplierSource = "default_overall"
	ResellerMultiplierSourceDefaultGroup    ResellerMultiplierSource = "default_group"
	ResellerMultiplierSourceCustomerOverall ResellerMultiplierSource = "customer_overall"
	ResellerMultiplierSourceCustomerGroup   ResellerMultiplierSource = "customer_group"
)

type ResellerResolvedMultiplier struct {
	MultiplierBps int                      `json:"multiplier_bps"`
	Source        ResellerMultiplierSource `json:"source"`
}

func ValidateResellerMultiplierBps(multiplierBps int) error {
	if multiplierBps < ResellerMultiplierBaseBps || multiplierBps > ResellerMultiplierMaxBps {
		return fmt.Errorf("%w: %d", ErrResellerMultiplierOutOfRange, multiplierBps)
	}
	return nil
}

func normalizeResellerPricingGroup(groupName string) string {
	return strings.TrimSpace(groupName)
}

func validateResellerPricingOwner(ownerType string, ownerId int64) error {
	if ownerId <= 0 {
		return ErrResellerPricingOwnerInvalid
	}
	if ownerType != ResellerPricingOwnerDefault && ownerType != ResellerPricingOwnerCustomer {
		return ErrResellerPricingOwnerInvalid
	}
	return nil
}

// ActiveResellerMultiplier returns the value effective at now without mutating
// persisted state. This makes request pricing correct even before the activation
// maintenance pass materializes a due increase.
func ActiveResellerMultiplier(rule ResellerPricingRule, now int64) int {
	if rule.PendingMultiplierBps > 0 && rule.PendingEffectiveAt > 0 && now >= rule.PendingEffectiveAt {
		return rule.PendingMultiplierBps
	}
	return rule.CurrentMultiplierBps
}

// PlanResellerPricingUpdate applies the observed activation state machine to a
// copy of the rule. New rules and decreases are immediate; increases wait 24h.
func PlanResellerPricingUpdate(current *ResellerPricingRule, multiplierBps int, now int64) (ResellerPricingRule, error) {
	if err := ValidateResellerMultiplierBps(multiplierBps); err != nil {
		return ResellerPricingRule{}, err
	}
	if current == nil {
		return ResellerPricingRule{
			CurrentMultiplierBps: multiplierBps,
			Version:              1,
		}, nil
	}

	next := *current
	active := ActiveResellerMultiplier(next, now)
	if next.PendingMultiplierBps > 0 && next.PendingEffectiveAt > 0 && now >= next.PendingEffectiveAt {
		next.CurrentMultiplierBps = active
		next.PendingMultiplierBps = 0
		next.PendingEffectiveAt = 0
	}

	if multiplierBps <= next.CurrentMultiplierBps {
		next.CurrentMultiplierBps = multiplierBps
		next.PendingMultiplierBps = 0
		next.PendingEffectiveAt = 0
	} else {
		next.PendingMultiplierBps = multiplierBps
		next.PendingEffectiveAt = now + ResellerPriceIncreaseWait
	}
	next.Version++
	return next, nil
}

func resellerRuleForGroup(rules map[string]ResellerPricingRule, groupName string) (ResellerPricingRule, bool) {
	rule, ok := rules[normalizeResellerPricingGroup(groupName)]
	return rule, ok
}

// ResolveResellerMultiplier implements the confirmed four-level precedence.
// Empty-string keys represent overall rules.
func ResolveResellerMultiplier(
	defaultRules map[string]ResellerPricingRule,
	customerRules map[string]ResellerPricingRule,
	groupName string,
	now int64,
) ResellerResolvedMultiplier {
	if rule, ok := resellerRuleForGroup(customerRules, groupName); ok {
		return ResellerResolvedMultiplier{ActiveResellerMultiplier(rule, now), ResellerMultiplierSourceCustomerGroup}
	}
	if rule, ok := customerRules[""]; ok {
		return ResellerResolvedMultiplier{ActiveResellerMultiplier(rule, now), ResellerMultiplierSourceCustomerOverall}
	}
	if rule, ok := resellerRuleForGroup(defaultRules, groupName); ok {
		return ResellerResolvedMultiplier{ActiveResellerMultiplier(rule, now), ResellerMultiplierSourceDefaultGroup}
	}
	if rule, ok := defaultRules[""]; ok {
		return ResellerResolvedMultiplier{ActiveResellerMultiplier(rule, now), ResellerMultiplierSourceDefaultOverall}
	}
	return ResellerResolvedMultiplier{ResellerMultiplierBaseBps, ResellerMultiplierSourceBase}
}

func resellerPricingVersionColumn(ownerType string) (model any, err error) {
	switch ownerType {
	case ResellerPricingOwnerDefault:
		return &ResellerProfile{}, nil
	case ResellerPricingOwnerCustomer:
		return &ResellerCustomer{}, nil
	default:
		return nil, ErrResellerPricingOwnerInvalid
	}
}

// UpdateResellerPricingRule performs an owner-wide optimistic-lock update and
// the corresponding rule mutation in one transaction.
func UpdateResellerPricingRule(
	ownerType string,
	ownerId int64,
	groupName string,
	multiplierBps int,
	expectedVersion int64,
	now int64,
) (*ResellerPricingRule, int64, error) {
	if err := validateResellerPricingOwner(ownerType, ownerId); err != nil {
		return nil, 0, err
	}
	if expectedVersion <= 0 {
		return nil, 0, ErrResellerPricingVersionConflict
	}
	if err := ValidateResellerMultiplierBps(multiplierBps); err != nil {
		return nil, 0, err
	}

	groupName = normalizeResellerPricingGroup(groupName)
	var result ResellerPricingRule
	newVersion := expectedVersion + 1
	err := DB.Transaction(func(tx *gorm.DB) error {
		ownerModel, err := resellerPricingVersionColumn(ownerType)
		if err != nil {
			return err
		}
		versionUpdate := tx.Model(ownerModel).
			Where("id = ? AND pricing_version = ?", ownerId, expectedVersion).
			Update("pricing_version", newVersion)
		if versionUpdate.Error != nil {
			return versionUpdate.Error
		}
		if versionUpdate.RowsAffected != 1 {
			return ErrResellerPricingVersionConflict
		}

		current := ResellerPricingRule{}
		findErr := tx.Where("owner_type = ? AND owner_id = ? AND group_name = ?", ownerType, ownerId, groupName).
			First(&current).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}

		var planned ResellerPricingRule
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			planned, err = PlanResellerPricingUpdate(nil, multiplierBps, now)
			planned.OwnerType = ownerType
			planned.OwnerId = ownerId
			planned.GroupName = groupName
		} else {
			planned, err = PlanResellerPricingUpdate(&current, multiplierBps, now)
		}
		if err != nil {
			return err
		}
		if err := tx.Save(&planned).Error; err != nil {
			return err
		}
		result = planned
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return &result, newVersion, nil
}

func GetResellerPricingRules(ownerType string, ownerId int64) (map[string]ResellerPricingRule, error) {
	if err := validateResellerPricingOwner(ownerType, ownerId); err != nil {
		return nil, err
	}
	var rows []ResellerPricingRule
	if err := DB.Where("owner_type = ? AND owner_id = ?", ownerType, ownerId).Find(&rows).Error; err != nil {
		return nil, err
	}
	rules := make(map[string]ResellerPricingRule, len(rows))
	for _, row := range rows {
		rules[row.GroupName] = row
	}
	return rules, nil
}
