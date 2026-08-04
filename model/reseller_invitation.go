package model

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ResellerInvitationStatusActive  = "active"
	ResellerInvitationStatusRevoked = "revoked"
	ResellerInvitationTokenPrefix   = "i1"
	resellerInvitationPublicBytes   = 16
)

var (
	ErrResellerInvitationInvalid = errors.New("reseller invitation is invalid")
	ErrResellerInvitationExpired = errors.New("reseller invitation has expired")
	ErrResellerCustomerBound     = errors.New("customer already belongs to a reseller")
	ErrResellerSelfBinding       = errors.New("reseller cannot bind itself as a customer")
)

// ResellerInvitation stores only a public random identifier and an HMAC of the
// complete token. The bearer token can be deterministically reconstructed while
// the raw token is never persisted.
type ResellerInvitation struct {
	Id         int64  `json:"id" gorm:"primaryKey"`
	ResellerId int    `json:"reseller_id" gorm:"not null;uniqueIndex:ux_reseller_invitations_reseller"`
	PublicId   string `json:"-" gorm:"type:varchar(32);not null;uniqueIndex:ux_reseller_invitations_public"`
	TokenHash  string `json:"-" gorm:"type:char(64);not null;uniqueIndex:ux_reseller_invitations_hash"`
	Status     string `json:"status" gorm:"type:varchar(16);not null;default:'active';index"`
	ExpiresAt  int64  `json:"expires_at" gorm:"type:bigint;not null;default:0;index"`
	Version    int64  `json:"version" gorm:"type:bigint;not null;default:1"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt  int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func resellerInvitationSigningKey() []byte {
	return []byte("reseller-invitation-v1:" + common.SessionSecret)
}

func resellerInvitationSignature(publicId string) string {
	mac := hmac.New(sha256.New, resellerInvitationSigningKey())
	_, _ = mac.Write([]byte(publicId))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func buildResellerInvitationToken(publicId string) string {
	return ResellerInvitationTokenPrefix + "." + publicId + "." + resellerInvitationSignature(publicId)
}

func resellerInvitationTokenHash(token string) string {
	return common.GenerateHMACWithKey(resellerInvitationSigningKey(), token)
}

func parseResellerInvitationToken(token string) (string, bool) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 || parts[0] != ResellerInvitationTokenPrefix || parts[1] == "" || parts[2] == "" {
		return "", false
	}
	expected := resellerInvitationSignature(parts[1])
	if !hmac.Equal([]byte(parts[2]), []byte(expected)) {
		return "", false
	}
	return parts[1], true
}

func newResellerInvitationPublicId() (string, error) {
	randomBytes := make([]byte, resellerInvitationPublicBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

// GetOrCreateResellerInvitation returns the stable invitation for an active
// reseller. Reconstructing the token also rotates its digest if the deployment
// secret changed, invalidating tokens signed by the previous secret.
func GetOrCreateResellerInvitation(resellerId int, now int64) (string, *ResellerInvitation, error) {
	if resellerId <= 0 {
		return "", nil, ErrResellerInvitationInvalid
	}
	var invitation ResellerInvitation
	var token string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var profile ResellerProfile
		if err := lockForUpdate(tx).Where("user_id = ? AND status = ?", resellerId, ResellerStatusActive).First(&profile).Error; err != nil {
			return ErrResellerInvitationInvalid
		}

		err := lockForUpdate(tx).Where("reseller_id = ?", resellerId).First(&invitation).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			publicId, generateErr := newResellerInvitationPublicId()
			if generateErr != nil {
				return generateErr
			}
			token = buildResellerInvitationToken(publicId)
			invitation = ResellerInvitation{
				ResellerId: resellerId,
				PublicId:   publicId,
				TokenHash:  resellerInvitationTokenHash(token),
				Status:     ResellerInvitationStatusActive,
			}
			return tx.Create(&invitation).Error
		}

		if invitation.Status != ResellerInvitationStatusActive ||
			(invitation.ExpiresAt > 0 && invitation.ExpiresAt <= now) {
			return ErrResellerInvitationExpired
		}
		token = buildResellerInvitationToken(invitation.PublicId)
		tokenHash := resellerInvitationTokenHash(token)
		if invitation.TokenHash != tokenHash {
			invitation.TokenHash = tokenHash
			invitation.Version++
			if err := tx.Model(&invitation).Updates(map[string]any{
				"token_hash": tokenHash,
				"version":    invitation.Version,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	return token, &invitation, nil
}

func resolveResellerInvitationWithTx(tx *gorm.DB, token string, now int64) (*ResellerInvitation, error) {
	if tx == nil {
		return nil, ErrResellerInvitationInvalid
	}
	publicId, ok := parseResellerInvitationToken(token)
	if !ok {
		return nil, ErrResellerInvitationInvalid
	}
	var invitation ResellerInvitation
	err := lockForUpdate(tx).
		Where("public_id = ? AND token_hash = ? AND status = ?", publicId, resellerInvitationTokenHash(strings.TrimSpace(token)), ResellerInvitationStatusActive).
		First(&invitation).Error
	if err != nil {
		return nil, ErrResellerInvitationInvalid
	}
	if invitation.ExpiresAt > 0 && invitation.ExpiresAt <= now {
		return nil, ErrResellerInvitationExpired
	}
	var profile ResellerProfile
	if err := tx.Where("user_id = ? AND status = ?", invitation.ResellerId, ResellerStatusActive).First(&profile).Error; err != nil {
		return nil, ErrResellerInvitationInvalid
	}
	return &invitation, nil
}

func ResolveResellerInvitation(token string, now int64) (*ResellerInvitation, error) {
	var invitation *ResellerInvitation
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		invitation, err = resolveResellerInvitationWithTx(tx, token, now)
		return err
	})
	return invitation, err
}

func resolveResellerInvitationReferenceWithTx(tx *gorm.DB, invitationId int64, version int64, now int64) (*ResellerInvitation, error) {
	if tx == nil || invitationId <= 0 || version <= 0 {
		return nil, ErrResellerInvitationInvalid
	}
	var invitation ResellerInvitation
	if err := lockForUpdate(tx).
		Where("id = ? AND version = ? AND status = ?", invitationId, version, ResellerInvitationStatusActive).
		First(&invitation).Error; err != nil {
		return nil, ErrResellerInvitationInvalid
	}
	if invitation.ExpiresAt > 0 && invitation.ExpiresAt <= now {
		return nil, ErrResellerInvitationExpired
	}
	var profile ResellerProfile
	if err := tx.Where("user_id = ? AND status = ?", invitation.ResellerId, ResellerStatusActive).First(&profile).Error; err != nil {
		return nil, ErrResellerInvitationInvalid
	}
	return &invitation, nil
}

func ResolveResellerInvitationReference(invitationId int64, version int64, now int64) (*ResellerInvitation, error) {
	var invitation *ResellerInvitation
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		invitation, err = resolveResellerInvitationReferenceWithTx(tx, invitationId, version, now)
		return err
	})
	return invitation, err
}

func validateResellerRegistrationSource(source string) bool {
	switch source {
	case ResellerRegistrationSourcePrimary,
		ResellerRegistrationSourceReseller,
		ResellerRegistrationSourceAdmin,
		ResellerRegistrationSourceLegacyUnknown:
		return true
	default:
		return false
	}
}

func createResellerCustomerBindingWithTx(tx *gorm.DB, resellerId int, customerId int, source string, now int64) (*ResellerCustomer, error) {
	if tx == nil || resellerId <= 0 || customerId <= 0 || !validateResellerRegistrationSource(source) {
		return nil, ErrResellerInvitationInvalid
	}
	if resellerId == customerId {
		return nil, ErrResellerSelfBinding
	}

	var customer User
	if err := lockForUpdate(tx).First(&customer, customerId).Error; err != nil {
		return nil, err
	}
	if customer.InviterId != 0 && customer.InviterId != resellerId {
		return nil, ErrResellerCustomerBound
	}
	var existing ResellerCustomer
	err := lockForUpdate(tx).Where("customer_id = ?", customerId).First(&existing).Error
	if err == nil {
		if existing.ResellerId == resellerId {
			return &existing, nil
		}
		return nil, ErrResellerCustomerBound
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	binding := &ResellerCustomer{
		ResellerId:         resellerId,
		CustomerId:         customerId,
		RegistrationSource: source,
		BoundAt:            now,
	}
	createResult := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "customer_id"}},
		DoNothing: true,
	}).Create(binding)
	if createResult.Error != nil {
		return nil, createResult.Error
	}
	if createResult.RowsAffected != 1 {
		if err := tx.Where("customer_id = ?", customerId).First(&existing).Error; err != nil {
			return nil, err
		}
		if existing.ResellerId != resellerId {
			return nil, ErrResellerCustomerBound
		}
		return &existing, nil
	}
	if customer.InviterId == 0 {
		if err := tx.Model(&User{}).Where("id = ? AND inviter_id = ?", customerId, 0).Update("inviter_id", resellerId).Error; err != nil {
			return nil, err
		}
	}
	return binding, nil
}

// BindResellerCustomerFromInvitationWithTx revalidates the invitation and
// writes the ownership edge in the caller's user-creation transaction.
func BindResellerCustomerFromInvitationWithTx(tx *gorm.DB, token string, customerId int, source string, now int64) (*ResellerCustomer, error) {
	invitation, err := resolveResellerInvitationWithTx(tx, token, now)
	if err != nil {
		return nil, err
	}
	return createResellerCustomerBindingWithTx(tx, invitation.ResellerId, customerId, source, now)
}

func BindResellerCustomerFromInvitationReferenceWithTx(
	tx *gorm.DB,
	invitationId int64,
	version int64,
	customerId int,
	source string,
	now int64,
) (*ResellerCustomer, error) {
	invitation, err := resolveResellerInvitationReferenceWithTx(tx, invitationId, version, now)
	if err != nil {
		return nil, err
	}
	return createResellerCustomerBindingWithTx(tx, invitation.ResellerId, customerId, source, now)
}
