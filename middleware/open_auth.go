package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

const (
	// ContextKeyOpenApp holds the authenticated *model.OpenApp.
	ContextKeyOpenApp = "open_app"
	// ContextKeyOpenAppId holds the authenticated partner's app id, set by both
	// app authentication and credential authentication.
	ContextKeyOpenAppId = "open_app_id"
	// ContextKeyOpenCredentialId holds the row id of the presented credential.
	ContextKeyOpenCredentialId = "open_credential_id"

	openAppIdHeader     = "X-App-Id"
	openAppSecretHeader = "X-App-Secret"

	// openCredentialTouchInterval throttles last-used bookkeeping so a busy
	// partner does not turn every balance read into a database write.
	openCredentialTouchInterval = 60
)

// Open API error codes. These are part of the published integration contract:
// partners branch on the code, so the strings must stay stable even if the
// human-readable message changes.
const (
	OpenErrDisabled           = "OPEN_API_DISABLED"
	OpenErrAppUnauthorized    = "APP_UNAUTHORIZED"
	OpenErrAppDisabled        = "APP_DISABLED"
	OpenErrAppIpNotAllowed    = "APP_IP_NOT_ALLOWED"
	OpenErrCredentialInvalid  = "CREDENTIAL_INVALID"
	OpenErrCredentialRevoked  = "CREDENTIAL_REVOKED"
	OpenErrUserDisabled       = "USER_DISABLED"
	OpenErrRateLimited        = "RATE_LIMITED"
	OpenErrInternal           = "INTERNAL_ERROR"
	OpenErrInvalidParams      = "INVALID_PARAMS"
	OpenErrInvalidCredentials = "INVALID_CREDENTIALS"
	OpenErrRequire2FA         = "REQUIRE_2FA_UNSUPPORTED"
)

// AbortOpenApi ends an open API request with the machine-readable envelope the
// integration contract documents. Messages are intentionally fixed English
// rather than i18n: a partner's server parses them, and locale-varying text
// would make their error handling non-deterministic.
func AbortOpenApi(c *gin.Context, status int, code string, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"success": false,
		"code":    code,
		"message": message,
	})
}

// OpenApiEnabled gates the whole namespace behind an explicit operator opt-in,
// so the endpoints stay dark until someone deliberately turns them on.
func OpenApiEnabled() func(c *gin.Context) {
	return func(c *gin.Context) {
		if !system_setting.GetOpenBalanceApiSettings().Enabled {
			AbortOpenApi(c, http.StatusServiceUnavailable, OpenErrDisabled,
				"The balance open API is disabled on this site.")
			return
		}
		c.Next()
	}
}

// OpenAppAuth authenticates the partner site itself. It must run before any
// end-user credentials are accepted, so an anonymous caller can never reach the
// password comparison.
func OpenAppAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		appId := strings.TrimSpace(c.GetHeader(openAppIdHeader))
		secret := strings.TrimSpace(c.GetHeader(openAppSecretHeader))
		if appId == "" || secret == "" {
			AbortOpenApi(c, http.StatusUnauthorized, OpenErrAppUnauthorized,
				"Missing X-App-Id or X-App-Secret header.")
			return
		}
		app, err := model.ValidateOpenApp(appId, secret, c.ClientIP())
		if err != nil {
			switch {
			case errors.Is(err, model.ErrOpenAppUnauthorized):
				AbortOpenApi(c, http.StatusUnauthorized, OpenErrAppUnauthorized,
					"The application credentials are invalid.")
			case errors.Is(err, model.ErrOpenAppDisabled):
				AbortOpenApi(c, http.StatusForbidden, OpenErrAppDisabled,
					"The application has been disabled.")
			case errors.Is(err, model.ErrOpenAppIpNotAllowed):
				AbortOpenApi(c, http.StatusForbidden, OpenErrAppIpNotAllowed,
					"The request source IP is not allowed for this application.")
			default:
				common.SysLog("open app authentication error: " + err.Error())
				AbortOpenApi(c, http.StatusInternalServerError, OpenErrInternal,
					"Internal error while authenticating the application.")
			}
			return
		}
		c.Set(ContextKeyOpenApp, app)
		c.Set(ContextKeyOpenAppId, app.AppId)
		c.Next()
	}
}

// OpenCredentialAuth authenticates the read-only bearer credential a user
// granted to a partner. It resolves the owning user into the standard "id"
// context key so downstream handlers read like any other authenticated route.
func OpenCredentialAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		raw, ok := authorizationToken(c.GetHeader("Authorization"))
		if !ok {
			AbortOpenApi(c, http.StatusUnauthorized, OpenErrCredentialInvalid,
				"Missing bearer credential.")
			return
		}
		credential, user, err := model.ValidateOpenCredential(raw)
		if err != nil {
			switch {
			case errors.Is(err, model.ErrOpenCredentialInvalid):
				AbortOpenApi(c, http.StatusUnauthorized, OpenErrCredentialInvalid,
					"The credential is invalid.")
			case errors.Is(err, model.ErrOpenCredentialRevoked):
				AbortOpenApi(c, http.StatusUnauthorized, OpenErrCredentialRevoked,
					"The credential has been revoked. Ask the user to authorize again.")
			case errors.Is(err, model.ErrOpenCredentialUserOff):
				AbortOpenApi(c, http.StatusForbidden, OpenErrUserDisabled,
					"The account is disabled.")
			default:
				common.SysLog("open credential authentication error: " + err.Error())
				AbortOpenApi(c, http.StatusInternalServerError, OpenErrInternal,
					"Internal error while validating the credential.")
			}
			return
		}
		c.Set("id", user.Id)
		c.Set("username", user.Username)
		c.Set(ContextKeyOpenAppId, credential.AppId)
		c.Set(ContextKeyOpenCredentialId, credential.Id)

		if common.GetTimestamp()-credential.LastUsedTime >= openCredentialTouchInterval {
			credentialId := credential.Id
			appId := credential.AppId
			gopool.Go(func() {
				model.TouchOpenCredentialLastUsed(credentialId)
				model.TouchOpenAppLastUsed(appId)
			})
		}
		c.Next()
	}
}

// GetOpenApp returns the partner authenticated by OpenAppAuth.
func GetOpenApp(c *gin.Context) *model.OpenApp {
	value, ok := c.Get(ContextKeyOpenApp)
	if !ok {
		return nil
	}
	app, ok := value.(*model.OpenApp)
	if !ok {
		return nil
	}
	return app
}
