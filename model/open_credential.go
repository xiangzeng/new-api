package model

import (
	"errors"
	"fmt"
	"strings"

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

	OpenCredentialPrefix           = "obk_"
	openCredentialRandomBytes      = 24
	openCredentialHintLength       = 6
	openCredentialUserAppMaxIssues = 3
)

var (
	ErrOpenCredentialInvalid = errors.New("open credential is invalid")
	ErrOpenCredentialRevoked = errors.New("open credential has been revoked")
	ErrOpenCredentialUserOff = errors.New("open credential owner is disabled")
)

// AuthenticateOpenApiUser resolves a username-or-email plus password to a user
// row for the balance open API.
//
// It mirrors User.ValidateAndFill but reports a disabled account separately.
// ValidateAndFill folds "banned" into "invalid credentials" so an anonymous
// login form cannot probe account status; here the caller has already proved
// knowledge of the password, so naming the real reason leaks nothing and saves
// the partner from showing a misleading "wrong password" message.
//
// Like ValidateAndFill, a miss returns before any bcrypt work, so response
// timing distinguishes existing from non-existing usernames. That oracle is
// accepted rather than paid for with a dummy-hash comparison: a per-request
// bcrypt on every miss would hand an authenticated partner a cheap CPU
// amplification vector, and the unauthenticated /api/user/login endpoint
// already exposes the same signal to everyone.
func AuthenticateOpenApiUser(username string, password string) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, ErrUserEmptyCredentials
	}
	var user User
	if err := DB.Where("username = ? OR email = ?", username, username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	if user.Password == "" || !common.ValidatePasswordAndHash(password, user.Password) {
		return nil, ErrInvalidCredentials
	}
	if user.Status != common.UserStatusEnabled {
		return nil, ErrOpenCredentialUserOff
	}
	return &user, nil
}

// OpenCredential is a long-lived, read-only bearer token a user granted to one
// third-party app. Only an HMAC digest is stored, so the clear-text value
// exists exactly once in the exchange response.
//
// AuthVersion pins the credential to the user's authentication generation.
// New API bumps auth_version on password change, status change and deletion,
// so those actions invalidate every outstanding credential with no extra
// bookkeeping on this side.
type OpenCredential struct {
	Id           int64  `json:"id" gorm:"primaryKey"`
	TokenHash    string `json:"-" gorm:"type:char(64);not null;uniqueIndex"`
	TokenHint    string `json:"token_hint" gorm:"type:varchar(32)"`
	UserId       int    `json:"user_id" gorm:"index:idx_open_credential_user_app"`
	AppId        string `json:"app_id" gorm:"type:varchar(64);index:idx_open_credential_user_app"`
	Scope        string `json:"scope" gorm:"type:varchar(64)"`
	Status       int    `json:"status" gorm:"index"`
	AuthVersion  int64  `json:"-" gorm:"type:bigint"`
	CreatedIp    string `json:"created_ip" gorm:"type:varchar(64)"`
	EndUserIp    string `json:"end_user_ip" gorm:"type:varchar(64)"`
	CreatedTime  int64  `json:"created_time"`
	LastUsedTime int64  `json:"last_used_time"`
	RevokedTime  int64  `json:"revoked_time"`
}

func (OpenCredential) TableName() string {
	return "open_credentials"
}

// OpenCredentialListItem is the user-facing view of a granted authorization.
// It deliberately carries no digest material, only what a person needs to
// recognize the grant and decide whether to revoke it.
type OpenCredentialListItem struct {
	Id           int64  `json:"id"`
	AppId        string `json:"app_id"`
	AppName      string `json:"app_name"`
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

// openCredentialSigningKey is a constant for the reasons documented on
// openAppSigningKey: the token it digests is already high-entropy random, and a
// process-dependent key would revoke every outstanding credential on restart.
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

// IssueOpenCredential mints a credential for one (user, app) pair and revokes
// the pair's previous credentials. Re-exchanging cannot return an existing
// clear-text value — it is not stored — so without this the table would grow a
// zombie row on every login the partner performs.
func IssueOpenCredential(userId int, appId string, authVersion int64, createdIp string, endUserIp string) (string, *OpenCredential, error) {
	if userId <= 0 || strings.TrimSpace(appId) == "" || authVersion <= 0 {
		return "", nil, ErrOpenCredentialInvalid
	}
	if _, err := RevokeOpenCredentialsByUserAndApp(userId, appId); err != nil {
		return "", nil, err
	}
	for range openCredentialUserAppMaxIssues {
		token, err := randomOpenToken(OpenCredentialPrefix, openCredentialRandomBytes)
		if err != nil {
			return "", nil, err
		}
		credential := &OpenCredential{
			TokenHash:   openCredentialHash(token),
			TokenHint:   openCredentialHint(token),
			UserId:      userId,
			AppId:       appId,
			Scope:       OpenScopeBalanceRead,
			Status:      OpenCredentialStatusActive,
			AuthVersion: authVersion,
			CreatedIp:   createdIp,
			EndUserIp:   endUserIp,
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

	app, err := GetOpenAppByAppId(credential.AppId)
	if err != nil {
		if errors.Is(err, ErrOpenAppNotFound) {
			return nil, nil, ErrOpenCredentialRevoked
		}
		return nil, nil, err
	}
	if app.Status != OpenAppStatusEnabled {
		return nil, nil, ErrOpenCredentialRevoked
	}

	user, err := GetUserCache(credential.UserId)
	if err != nil {
		return nil, nil, err
	}
	if user.Status != common.UserStatusEnabled {
		return nil, nil, ErrOpenCredentialUserOff
	}
	// A bumped auth_version means the password or account state changed after
	// this credential was granted; the third party must ask for consent again.
	if user.AuthVersion != credential.AuthVersion {
		return nil, nil, ErrOpenCredentialRevoked
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

func RevokeOpenCredentialsByUserAndApp(userId int, appId string) (int64, error) {
	return revokeOpenCredentialsWhere(
		DB.Model(&OpenCredential{}).Where("user_id = ? AND app_id = ?", userId, appId),
	)
}

func RevokeOpenCredentialsByApp(appId string) (int64, error) {
	if strings.TrimSpace(appId) == "" {
		return 0, nil
	}
	return revokeOpenCredentialsWhere(
		DB.Model(&OpenCredential{}).Where("app_id = ?", appId),
	)
}

// RevokeOpenCredentialByToken lets a partner drop the credential when the end
// user logs out of their site, without needing to know its row id.
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
// can never revoke another user's grant by guessing a row id.
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
	if len(credentials) == 0 {
		return items, nil
	}

	appIds := make([]string, 0, len(credentials))
	for _, credential := range credentials {
		appIds = append(appIds, credential.AppId)
	}
	var apps []OpenApp
	if err := DB.Select("app_id", "name").Where("app_id IN ?", appIds).Find(&apps).Error; err != nil {
		return nil, err
	}
	namesByAppId := make(map[string]string, len(apps))
	for _, app := range apps {
		namesByAppId[app.AppId] = app.Name
	}

	for _, credential := range credentials {
		items = append(items, OpenCredentialListItem{
			Id:           credential.Id,
			AppId:        credential.AppId,
			AppName:      namesByAppId[credential.AppId],
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
