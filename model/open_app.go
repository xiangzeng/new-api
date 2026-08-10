package model

import (
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	OpenAppStatusEnabled  = 1
	OpenAppStatusDisabled = 2

	OpenAppIdPrefix     = "oapp_"
	OpenAppSecretPrefix = "oas_"

	openAppIdRandomBytes     = 12
	openAppSecretRandomBytes = 24
	openAppNameMaxLength     = 64
	openAppHintLength        = 6
)

var (
	ErrOpenAppNotFound     = errors.New("open app not found")
	ErrOpenAppDisabled     = errors.New("open app is disabled")
	ErrOpenAppUnauthorized = errors.New("open app credentials are invalid")
	ErrOpenAppIpNotAllowed = errors.New("open app source ip is not allowed")
	ErrOpenAppNameInvalid  = errors.New("open app name is invalid")
)

// OpenApp is a third-party site allowed to exchange an end user's password for
// a read-only balance credential. The secret is never persisted in clear text:
// only an HMAC digest and a short trailing hint are stored, so a database leak
// cannot be replayed against the exchange endpoint.
type OpenApp struct {
	Id                int    `json:"id" gorm:"primaryKey"`
	AppId             string `json:"app_id" gorm:"type:varchar(64);uniqueIndex"`
	SecretHash        string `json:"-" gorm:"type:char(64);not null"`
	SecretHint        string `json:"secret_hint" gorm:"type:varchar(32)"`
	Name              string `json:"name" gorm:"type:varchar(64);not null"`
	Status            int    `json:"status" gorm:"index"`
	AllowedIps        string `json:"allowed_ips" gorm:"type:text"`
	ExchangeRateLimit int    `json:"exchange_rate_limit"`
	CreatedTime       int64  `json:"created_time"`
	LastUsedTime      int64  `json:"last_used_time"`
}

func (OpenApp) TableName() string {
	return "open_apps"
}

// openAppSigningKey is domain separated from every other HMAC in the project so
// a digest from one table can never validate against another.
//
// The key deliberately carries no secret. What it digests is already
// openAppSecretRandomBytes of crypto/rand output, so a secret key would buy
// nothing against an attacker who cannot brute force that entropy in the first
// place — while making the digest depend on process state. Deriving it from
// common.SessionSecret, which is re-rolled on every boot unless SESSION_SECRET
// is set, silently invalidated every stored secret on restart. A constant key is
// what makes an issued secret outlive a redeploy; rotation stays an explicit
// action through ResetOpenAppSecret.
func openAppSigningKey() []byte {
	return []byte("open-app-secret-v1")
}

func openAppSecretHash(secret string) string {
	return common.GenerateHMACWithKey(openAppSigningKey(), secret)
}

// openAppSecretHint keeps only the trailing characters of the secret so an
// operator can tell two credentials apart in the admin list without the row
// carrying enough material to reconstruct the secret.
func openAppSecretHint(secret string) string {
	if len(secret) <= openAppHintLength {
		return OpenAppSecretPrefix + "…"
	}
	return OpenAppSecretPrefix + "…" + secret[len(secret)-openAppHintLength:]
}

func randomOpenToken(prefix string, randomBytes int) (string, error) {
	buf := make([]byte, randomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate open api token: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// GetOpenAppAllowedIps parses the newline separated CIDR/IP allow list using the
// same lenient format as token IP limits, so operators do not have to learn a
// second syntax.
func (app *OpenApp) GetOpenAppAllowedIps() []string {
	allowed := make([]string, 0)
	cleaned := strings.ReplaceAll(app.AllowedIps, " ", "")
	if cleaned == "" {
		return allowed
	}
	for _, entry := range strings.Split(cleaned, "\n") {
		entry = strings.ReplaceAll(strings.TrimSpace(entry), ",", "")
		if entry != "" {
			allowed = append(allowed, entry)
		}
	}
	return allowed
}

func (app *OpenApp) CheckSourceIp(clientIp string) error {
	allowed := app.GetOpenAppAllowedIps()
	if len(allowed) == 0 {
		return nil
	}
	ip := net.ParseIP(clientIp)
	if ip == nil || !common.IsIpInCIDRList(ip, allowed) {
		return ErrOpenAppIpNotAllowed
	}
	return nil
}

func normalizeOpenAppName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > openAppNameMaxLength {
		return "", ErrOpenAppNameInvalid
	}
	return name, nil
}

// CreateOpenApp returns the freshly minted app together with its clear-text
// secret. The secret is returned exactly once and cannot be recovered later.
func CreateOpenApp(name string, allowedIps string, exchangeRateLimit int) (*OpenApp, string, error) {
	name, err := normalizeOpenAppName(name)
	if err != nil {
		return nil, "", err
	}
	if exchangeRateLimit < 0 {
		exchangeRateLimit = 0
	}
	secret, err := randomOpenToken(OpenAppSecretPrefix, openAppSecretRandomBytes)
	if err != nil {
		return nil, "", err
	}
	for range 3 {
		appId, err := randomOpenToken(OpenAppIdPrefix, openAppIdRandomBytes)
		if err != nil {
			return nil, "", err
		}
		app := &OpenApp{
			AppId:             appId,
			SecretHash:        openAppSecretHash(secret),
			SecretHint:        openAppSecretHint(secret),
			Name:              name,
			Status:            OpenAppStatusEnabled,
			AllowedIps:        strings.TrimSpace(allowedIps),
			ExchangeRateLimit: exchangeRateLimit,
			CreatedTime:       common.GetTimestamp(),
		}
		if err := DB.Create(app).Error; err == nil {
			return app, secret, nil
		}
	}
	return nil, "", errors.New("failed to allocate a unique open app id")
}

func GetOpenAppByAppId(appId string) (*OpenApp, error) {
	appId = strings.TrimSpace(appId)
	if appId == "" {
		return nil, ErrOpenAppNotFound
	}
	var app OpenApp
	if err := DB.Where("app_id = ?", appId).First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOpenAppNotFound
		}
		return nil, err
	}
	return &app, nil
}

func GetOpenAppById(id int) (*OpenApp, error) {
	if id <= 0 {
		return nil, ErrOpenAppNotFound
	}
	var app OpenApp
	if err := DB.Where("id = ?", id).First(&app).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOpenAppNotFound
		}
		return nil, err
	}
	return &app, nil
}

// ValidateOpenApp authenticates a partner site. An unknown app id and a wrong
// secret both surface as ErrOpenAppUnauthorized so the caller cannot enumerate
// which app ids exist, and the digest comparison is constant time.
func ValidateOpenApp(appId string, secret string, clientIp string) (*OpenApp, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrOpenAppUnauthorized
	}
	app, err := GetOpenAppByAppId(appId)
	if err != nil {
		if errors.Is(err, ErrOpenAppNotFound) {
			return nil, ErrOpenAppUnauthorized
		}
		return nil, err
	}
	if !hmac.Equal([]byte(app.SecretHash), []byte(openAppSecretHash(secret))) {
		return nil, ErrOpenAppUnauthorized
	}
	if app.Status != OpenAppStatusEnabled {
		return nil, ErrOpenAppDisabled
	}
	if err := app.CheckSourceIp(clientIp); err != nil {
		return nil, err
	}
	return app, nil
}

func ListOpenApps() ([]*OpenApp, error) {
	apps := make([]*OpenApp, 0)
	if err := DB.Order("id DESC").Find(&apps).Error; err != nil {
		return nil, err
	}
	return apps, nil
}

func UpdateOpenApp(id int, name string, allowedIps string, status int, exchangeRateLimit int) (*OpenApp, error) {
	app, err := GetOpenAppById(id)
	if err != nil {
		return nil, err
	}
	name, err = normalizeOpenAppName(name)
	if err != nil {
		return nil, err
	}
	if status != OpenAppStatusEnabled && status != OpenAppStatusDisabled {
		status = app.Status
	}
	if exchangeRateLimit < 0 {
		exchangeRateLimit = 0
	}
	updates := map[string]interface{}{
		"name":                name,
		"allowed_ips":         strings.TrimSpace(allowedIps),
		"status":              status,
		"exchange_rate_limit": exchangeRateLimit,
	}
	if err := DB.Model(&OpenApp{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	// Disabling a partner must also cut off the credentials it already holds,
	// otherwise previously issued tokens keep answering balance queries.
	if status == OpenAppStatusDisabled && app.Status != OpenAppStatusDisabled {
		if _, err := RevokeOpenCredentialsByApp(app.AppId); err != nil {
			return nil, err
		}
	}
	return GetOpenAppById(id)
}

// ResetOpenAppSecret rotates the secret and revokes every credential issued
// under the old one, so a suspected leak can be contained in a single action.
func ResetOpenAppSecret(id int) (*OpenApp, string, error) {
	app, err := GetOpenAppById(id)
	if err != nil {
		return nil, "", err
	}
	secret, err := randomOpenToken(OpenAppSecretPrefix, openAppSecretRandomBytes)
	if err != nil {
		return nil, "", err
	}
	updates := map[string]interface{}{
		"secret_hash": openAppSecretHash(secret),
		"secret_hint": openAppSecretHint(secret),
	}
	if err := DB.Model(&OpenApp{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, "", err
	}
	if _, err := RevokeOpenCredentialsByApp(app.AppId); err != nil {
		return nil, "", err
	}
	updated, err := GetOpenAppById(id)
	if err != nil {
		return nil, "", err
	}
	return updated, secret, nil
}

func DeleteOpenApp(id int) error {
	app, err := GetOpenAppById(id)
	if err != nil {
		return err
	}
	if _, err := RevokeOpenCredentialsByApp(app.AppId); err != nil {
		return err
	}
	return DB.Where("id = ?", id).Delete(&OpenApp{}).Error
}

func TouchOpenAppLastUsed(appId string) {
	if appId == "" {
		return
	}
	if err := DB.Model(&OpenApp{}).Where("app_id = ?", appId).
		Update("last_used_time", common.GetTimestamp()).Error; err != nil {
		common.SysLog("failed to update open app last used time: " + err.Error())
	}
}
