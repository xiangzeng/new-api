package model

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	OpenCredentialStatusActive  = 1
	OpenCredentialStatusRevoked = 2

	// OpenScopeBalanceRead is the only scope currently issued. It is persisted
	// per row so a future scope can be added without invalidating existing
	// credentials or widening what the ones already in the wild can reach.
	OpenScopeBalanceRead = "balance:read"

	OpenCredentialPrefix      = "obk_"
	openCredentialRandomBytes = 24
	openCredentialHintLength  = 6
	openCredentialMaxAttempts = 3

	// OpenCredentialMaxPerUser bounds how many active keys one account can hold.
	// Several are legitimate — one per program the user runs — but the ceiling
	// keeps a scripted loop from filling the table.
	OpenCredentialMaxPerUser = 5

	openCredentialNameMaxLength = 50
	openCredentialDefaultName   = "Balance key"
)

var (
	ErrOpenCredentialInvalid      = errors.New("open credential is invalid")
	ErrOpenCredentialRevoked      = errors.New("open credential has been revoked")
	ErrOpenCredentialUserOff      = errors.New("open credential owner is disabled")
	ErrOpenCredentialNameTooLong  = errors.New("open credential name is too long")
	ErrOpenCredentialLimitReached = errors.New("open credential limit reached")
)

// OpenCredential is a long-lived, read-only balance key a user issued to
// themselves. Only an HMAC digest is stored, so the clear-text value exists
// exactly once, in the response that created it.
//
// The row is deliberately not pinned to the owner's auth_version: this is the
// user's own artifact rather than a grant delegated to someone else, so it
// follows relay API keys and survives a password change. Disabling or deleting
// the account still invalidates it, because validation reads live user state.
type OpenCredential struct {
	Id           int64  `json:"id" gorm:"primaryKey"`
	TokenHash    string `json:"-" gorm:"type:char(64);not null;uniqueIndex"`
	TokenHint    string `json:"token_hint" gorm:"type:varchar(32)"`
	Name         string `json:"name" gorm:"type:varchar(64)"`
	UserId       int    `json:"user_id" gorm:"index"`
	Scope        string `json:"scope" gorm:"type:varchar(64)"`
	Status       int    `json:"status" gorm:"index"`
	CreatedIp    string `json:"created_ip" gorm:"type:varchar(64)"`
	CreatedTime  int64  `json:"created_time"`
	LastUsedTime int64  `json:"last_used_time"`
	RevokedTime  int64  `json:"revoked_time"`
}

func (OpenCredential) TableName() string {
	return "open_credentials"
}

// OpenCredentialListItem is the owner-facing view of a key. It deliberately
// carries no digest material, only what a person needs to recognize the key and
// decide whether to revoke it.
type OpenCredentialListItem struct {
	Id           int64  `json:"id"`
	Name         string `json:"name"`
	TokenHint    string `json:"token_hint"`
	Scope        string `json:"scope"`
	CreatedTime  int64  `json:"created_time"`
	LastUsedTime int64  `json:"last_used_time"`
}

// OpenBalanceSnapshot is the balance projection returned by the open API,
// read in a single query so the four numbers always describe the same instant.
type OpenBalanceSnapshot struct {
	Username     string `gorm:"column:username"`
	DisplayName  string `gorm:"column:display_name"`
	Quota        int    `gorm:"column:quota"`
	UsedQuota    int    `gorm:"column:used_quota"`
	RequestCount int    `gorm:"column:request_count"`
}

// openCredentialSigningKey is a constant rather than a process-derived value:
// the token it digests is already high-entropy random, and a key tied to the
// process would revoke every outstanding credential on restart.
func openCredentialSigningKey() []byte {
	return []byte("open-credential-v1")
}

func openCredentialHash(token string) string {
	return common.GenerateHMACWithKey(openCredentialSigningKey(), token)
}

func openCredentialHint(token string) string {
	if len(token) <= openCredentialHintLength {
		return OpenCredentialPrefix + "…"
	}
	return OpenCredentialPrefix + "…" + token[len(token)-openCredentialHintLength:]
}

func randomOpenToken(prefix string, randomBytes int) (string, error) {
	buf := make([]byte, randomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate open api token: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

func normalizeOpenCredentialName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return openCredentialDefaultName, nil
	}
	if utf8.RuneCountInString(name) > openCredentialNameMaxLength {
		return "", ErrOpenCredentialNameTooLong
	}
	return name, nil
}

// IssueOpenCredential mints a balance key for one user and returns its
// clear-text value, which is never stored and cannot be retrieved again.
//
// The active-key count is checked before insert rather than enforced by a
// constraint: the ceiling is a courtesy bound on the owner's own account, and
// the only way to race it is to race yourself through a rate-limited endpoint.
func IssueOpenCredential(userId int, name string, createdIp string) (string, *OpenCredential, error) {
	if userId <= 0 {
		return "", nil, ErrOpenCredentialInvalid
	}
	name, err := normalizeOpenCredentialName(name)
	if err != nil {
		return "", nil, err
	}
	var active int64
	if err := DB.Model(&OpenCredential{}).
		Where("user_id = ? AND status = ?", userId, OpenCredentialStatusActive).
		Count(&active).Error; err != nil {
		return "", nil, err
	}
	if active >= OpenCredentialMaxPerUser {
		return "", nil, ErrOpenCredentialLimitReached
	}
	for range openCredentialMaxAttempts {
		token, err := randomOpenToken(OpenCredentialPrefix, openCredentialRandomBytes)
		if err != nil {
			return "", nil, err
		}
		credential := &OpenCredential{
			TokenHash:   openCredentialHash(token),
			TokenHint:   openCredentialHint(token),
			Name:        name,
			UserId:      userId,
			Scope:       OpenScopeBalanceRead,
			Status:      OpenCredentialStatusActive,
			CreatedIp:   createdIp,
			CreatedTime: common.GetTimestamp(),
		}
		if err := DB.Create(credential).Error; err == nil {
			return token, credential, nil
		}
	}
	return "", nil, errors.New("failed to allocate a unique open credential")
}

// ValidateOpenCredential resolves a bearer token to its owner. Every rejection
// path returns ErrOpenCredentialInvalid or ErrOpenCredentialRevoked rather than
// leaking whether the digest matched a row at all.
func ValidateOpenCredential(token string) (*OpenCredential, *UserBase, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil, ErrOpenCredentialInvalid
	}
	var credential OpenCredential
	if err := DB.Where("token_hash = ?", openCredentialHash(token)).First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrOpenCredentialInvalid
		}
		return nil, nil, err
	}
	if credential.Status != OpenCredentialStatusActive {
		return nil, nil, ErrOpenCredentialRevoked
	}
	if credential.Scope != OpenScopeBalanceRead {
		return nil, nil, ErrOpenCredentialInvalid
	}

	user, err := GetUserCache(credential.UserId)
	if err != nil {
		return nil, nil, err
	}
	if user.Status != common.UserStatusEnabled {
		return nil, nil, ErrOpenCredentialUserOff
	}
	return &credential, user, nil
}

func GetOpenBalanceSnapshot(userId int) (*OpenBalanceSnapshot, error) {
	if userId <= 0 {
		return nil, ErrOpenCredentialInvalid
	}
	var snapshot OpenBalanceSnapshot
	err := DB.Model(&User{}).
		Select("username", "display_name", "quota", "used_quota", "request_count").
		Where("id = ?", userId).
		First(&snapshot).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOpenCredentialInvalid
		}
		return nil, err
	}
	return &snapshot, nil
}

func revokeOpenCredentialsWhere(query *gorm.DB) (int64, error) {
	result := query.Where("status = ?", OpenCredentialStatusActive).
		Updates(map[string]interface{}{
			"status":       OpenCredentialStatusRevoked,
			"revoked_time": common.GetTimestamp(),
		})
	return result.RowsAffected, result.Error
}

// RevokeOpenCredentialByToken lets the program holding a key retire it without
// knowing its row id, so a script can clean up after itself.
func RevokeOpenCredentialByToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrOpenCredentialInvalid
	}
	affected, err := revokeOpenCredentialsWhere(
		DB.Model(&OpenCredential{}).Where("token_hash = ?", openCredentialHash(token)),
	)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrOpenCredentialInvalid
	}
	return nil
}

// RevokeOpenCredentialByUser scopes the revoke to the owning user so one user
// can never revoke another user's key by guessing a row id.
func RevokeOpenCredentialByUser(userId int, id int64) error {
	if userId <= 0 || id <= 0 {
		return ErrOpenCredentialInvalid
	}
	affected, err := revokeOpenCredentialsWhere(
		DB.Model(&OpenCredential{}).Where("id = ? AND user_id = ?", id, userId),
	)
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrOpenCredentialInvalid
	}
	return nil
}

func ListOpenCredentialsByUser(userId int) ([]OpenCredentialListItem, error) {
	items := make([]OpenCredentialListItem, 0)
	if userId <= 0 {
		return items, nil
	}
	var credentials []OpenCredential
	err := DB.Where("user_id = ? AND status = ?", userId, OpenCredentialStatusActive).
		Order("created_time DESC, id DESC").
		Find(&credentials).Error
	if err != nil {
		return nil, err
	}
	for _, credential := range credentials {
		items = append(items, OpenCredentialListItem{
			Id:           credential.Id,
			Name:         credential.Name,
			TokenHint:    credential.TokenHint,
			Scope:        credential.Scope,
			CreatedTime:  credential.CreatedTime,
			LastUsedTime: credential.LastUsedTime,
		})
	}
	return items, nil
}

func TouchOpenCredentialLastUsed(id int64) {
	if id <= 0 {
		return
	}
	if err := DB.Model(&OpenCredential{}).Where("id = ?", id).
		Update("last_used_time", common.GetTimestamp()).Error; err != nil {
		common.SysLog("failed to update open credential last used time: " + err.Error())
	}
}
