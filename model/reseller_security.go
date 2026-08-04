package model

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"regexp"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const ResellerOutboundFreezeSeconds int64 = 24 * 60 * 60

var (
	ErrResellerQuotaPasswordInvalid    = errors.New("reseller quota password is invalid")
	ErrResellerQuotaPasswordConfigured = errors.New("reseller quota password is already configured")
	ErrResellerQuotaPasswordMissing    = errors.New("reseller quota password is not configured")
	ErrResellerLoginPasswordInvalid    = errors.New("reseller login password is invalid")
	ErrResellerOutboundFrozen          = errors.New("reseller outbound operations are frozen")
	quotaPasswordPattern               = regexp.MustCompile(`^[0-9]{6}$`)
)

func ValidateResellerQuotaPassword(password string) error {
	if !quotaPasswordPattern.MatchString(password) {
		return ErrResellerQuotaPasswordInvalid
	}
	return nil
}

func SetResellerQuotaPassword(userId int, password string, now int64) (*ResellerSecurity, error) {
	if userId <= 0 || ValidateResellerQuotaPassword(password) != nil {
		return nil, ErrResellerQuotaPasswordInvalid
	}
	hash, err := common.Password2Hash(password)
	if err != nil {
		return nil, err
	}
	security := ResellerSecurity{UserId: userId, QuotaPasswordHash: hash, PasswordUpdatedAt: now}
	if err := DB.Create(&security).Error; err != nil {
		var existing ResellerSecurity
		if findErr := DB.Where("user_id = ?", userId).First(&existing).Error; findErr == nil {
			return nil, ErrResellerQuotaPasswordConfigured
		}
		return nil, err
	}
	return &security, nil
}

func ChangeResellerQuotaPassword(userId int, currentPassword string, newPassword string, now int64) error {
	if ValidateResellerQuotaPassword(newPassword) != nil {
		return ErrResellerQuotaPasswordInvalid
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var security ResellerSecurity
		if err := lockForUpdate(tx).Where("user_id = ?", userId).First(&security).Error; err != nil {
			return ErrResellerQuotaPasswordMissing
		}
		if !common.ValidatePasswordAndHash(currentPassword, security.QuotaPasswordHash) {
			return ErrResellerQuotaPasswordInvalid
		}
		hash, err := common.Password2Hash(newPassword)
		if err != nil {
			return err
		}
		return tx.Model(&security).Updates(map[string]any{
			"quota_password_hash": hash, "password_version": security.PasswordVersion + 1, "password_updated_at": now,
		}).Error
	})
}

func ResetResellerQuotaPassword(userId int, newPassword string, now int64) error {
	if ValidateResellerQuotaPassword(newPassword) != nil {
		return ErrResellerQuotaPasswordInvalid
	}
	hash, err := common.Password2Hash(newPassword)
	if err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var security ResellerSecurity
		err := lockForUpdate(tx).Where("user_id = ?", userId).First(&security).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			security = ResellerSecurity{UserId: userId, QuotaPasswordHash: hash, PasswordUpdatedAt: now, OutboundFrozenUntil: now + ResellerOutboundFreezeSeconds}
			return tx.Create(&security).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&security).Updates(map[string]any{
			"quota_password_hash": hash, "password_version": security.PasswordVersion + 1,
			"password_updated_at": now, "outbound_frozen_until": now + ResellerOutboundFreezeSeconds,
		}).Error
	})
}

func VerifyResellerQuotaPassword(userId int, password string, now int64, requireOutbound bool) error {
	var security ResellerSecurity
	if err := DB.Where("user_id = ?", userId).First(&security).Error; err != nil {
		return ErrResellerQuotaPasswordMissing
	}
	if !common.ValidatePasswordAndHash(password, security.QuotaPasswordHash) {
		return ErrResellerQuotaPasswordInvalid
	}
	if requireOutbound && security.OutboundFrozenUntil > now {
		return ErrResellerOutboundFrozen
	}
	return nil
}

func resellerSecretDigest(domain string, value string) string {
	return common.GenerateHMACWithKey([]byte(domain+":"+common.SessionSecret), value)
}

func encryptResellerSecret(plaintext string) (string, error) {
	key := sha256.Sum256([]byte("reseller-secret-v1:" + common.SessionSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), []byte("reseller-secret-v1"))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func decryptResellerSecret(ciphertext string) (string, error) {
	key := sha256.Sum256([]byte("reseller-secret-v1:" + common.SessionSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	sealed, err := base64.RawURLEncoding.DecodeString(ciphertext)
	if err != nil || len(sealed) < gcm.NonceSize() {
		return "", ErrResellerQuotaPasswordInvalid
	}
	nonce, encrypted := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, encrypted, []byte("reseller-secret-v1"))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func resellerNow(now int64) int64 {
	if now > 0 {
		return now
	}
	return time.Now().Unix()
}
