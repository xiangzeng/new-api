package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// OpenExchangeRequest is the body a partner backend posts to trade an end
// user's password for a read-only balance credential. EndUserIp is optional and
// used only for audit trail: every request reaches us from the partner's own
// server, so ClientIP alone cannot identify who actually authorized the grant.
type OpenExchangeRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	EndUserIp string `json:"end_user_ip"`
}

type OpenUserBrief struct {
	Id          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type OpenCredentialResponse struct {
	Credential string        `json:"credential"`
	Scope      string        `json:"scope"`
	User       OpenUserBrief `json:"user"`
}

type OpenBalanceResponse struct {
	UserId         int     `json:"user_id"`
	Username       string  `json:"username"`
	DisplayName    string  `json:"display_name"`
	Quota          int     `json:"quota"`
	UsedQuota      int     `json:"used_quota"`
	Balance        float64 `json:"balance"`
	Used           float64 `json:"used"`
	DisplayType    string  `json:"display_type"`
	CurrencySymbol string  `json:"currency_symbol"`
	RequestCount   int     `json:"request_count"`
}

// openQuotaAmount converts raw quota units into the site's configured display
// currency. It follows the same rules as the OpenAI-compatible billing
// endpoints so a partner and the dashboard never disagree about a balance, and
// additionally honors the CUSTOM display type via GetUsdToCurrencyRate.
func openQuotaAmount(quota int) float64 {
	amount := float64(quota)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		return amount
	}
	return amount / common.QuotaPerUnit * operation_setting.GetUsdToCurrencyRate(operation_setting.USDExchangeRate)
}

// OpenExchangeCredential trades an end user's password for a long-lived,
// balance-read-only credential scoped to the calling partner.
func OpenExchangeCredential(c *gin.Context) {
	app := middleware.GetOpenApp(c)
	if app == nil {
		middleware.AbortOpenApi(c, http.StatusUnauthorized, middleware.OpenErrAppUnauthorized,
			"The application credentials are invalid.")
		return
	}

	var request OpenExchangeRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		middleware.AbortOpenApi(c, http.StatusBadRequest, middleware.OpenErrInvalidParams,
			"Malformed JSON body.")
		return
	}
	username := strings.TrimSpace(request.Username)
	if username == "" || request.Password == "" {
		middleware.AbortOpenApi(c, http.StatusBadRequest, middleware.OpenErrInvalidParams,
			"Both username and password are required.")
		return
	}

	ctx := c.Request.Context()
	locked, retryAfter, err := middleware.OpenExchangeLockState(ctx, app.AppId, username)
	if err != nil {
		common.SysLog("open exchange lock check failed: " + err.Error())
		middleware.AbortOpenApi(c, http.StatusInternalServerError, middleware.OpenErrInternal,
			"Internal error while checking the lockout state.")
		return
	}
	if locked {
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.FormatInt(retryAfter, 10))
		}
		middleware.AbortOpenApi(c, http.StatusTooManyRequests, middleware.OpenErrRateLimited,
			"Too many failed attempts for this account, please retry later.")
		return
	}

	user, err := model.AuthenticateOpenApiUser(username, request.Password)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrInvalidCredentials), errors.Is(err, model.ErrUserEmptyCredentials):
			// Only a wrong password advances the lockout. A disabled account
			// supplied the right password, so counting it would let a banned
			// user lock themselves out of an eventual reinstatement.
			middleware.RecordOpenExchangeFailure(ctx, app.AppId, username)
			middleware.AbortOpenApi(c, http.StatusUnauthorized, middleware.OpenErrInvalidCredentials,
				"Incorrect username or password.")
		case errors.Is(err, model.ErrOpenCredentialUserOff):
			middleware.AbortOpenApi(c, http.StatusForbidden, middleware.OpenErrUserDisabled,
				"The account is disabled.")
		default:
			common.SysLog("open exchange authentication error: " + err.Error())
			middleware.AbortOpenApi(c, http.StatusInternalServerError, middleware.OpenErrInternal,
				"Internal error while verifying the account.")
		}
		return
	}

	twoFAEnabled, err := model.IsTwoFAEnabled(user.Id)
	if err != nil {
		common.SysLog("open exchange 2FA lookup error: " + err.Error())
		middleware.AbortOpenApi(c, http.StatusInternalServerError, middleware.OpenErrInternal,
			"Internal error while checking two-factor status.")
		return
	}
	if twoFAEnabled {
		// A password alone is not the full factor set this account chose, so
		// accepting it here would be a downgrade around the user's own decision.
		middleware.AbortOpenApi(c, http.StatusForbidden, middleware.OpenErrRequire2FA,
			"This account has two-factor authentication enabled and cannot authorize third-party balance queries. Please use the official dashboard.")
		return
	}

	credential, _, err := model.IssueOpenCredential(
		user.Id,
		app.AppId,
		user.AuthVersion,
		c.ClientIP(),
		strings.TrimSpace(request.EndUserIp),
	)
	if err != nil {
		common.SysLog("failed to issue open credential: " + err.Error())
		middleware.AbortOpenApi(c, http.StatusInternalServerError, middleware.OpenErrInternal,
			"Internal error while issuing the credential.")
		return
	}

	middleware.ClearOpenExchangeFailures(ctx, app.AppId, username)
	model.TouchOpenAppLastUsed(app.AppId)
	model.RecordLoginLog(
		user.Id,
		user.Username,
		"Authorized a third-party balance query credential for "+app.Name,
		c.ClientIP(),
		"open_api_exchange",
		map[string]interface{}{"app_id": app.AppId, "app_name": app.Name},
		map[string]interface{}{
			"login_method": "open_api",
			"user_agent":   c.Request.UserAgent(),
			"end_user_ip":  strings.TrimSpace(request.EndUserIp),
		},
	)

	common.ApiSuccess(c, OpenCredentialResponse{
		Credential: credential,
		Scope:      model.OpenScopeBalanceRead,
		User: OpenUserBrief{
			Id:          user.Id,
			Username:    user.Username,
			DisplayName: user.DisplayName,
		},
	})
}

// OpenGetBalance returns the credential owner's wallet balance.
func OpenGetBalance(c *gin.Context) {
	userId := c.GetInt("id")
	snapshot, err := model.GetOpenBalanceSnapshot(userId)
	if err != nil {
		if errors.Is(err, model.ErrOpenCredentialInvalid) {
			middleware.AbortOpenApi(c, http.StatusUnauthorized, middleware.OpenErrCredentialInvalid,
				"The credential is invalid.")
			return
		}
		common.SysLog("open balance query error: " + err.Error())
		middleware.AbortOpenApi(c, http.StatusInternalServerError, middleware.OpenErrInternal,
			"Internal error while reading the balance.")
		return
	}

	common.ApiSuccess(c, OpenBalanceResponse{
		UserId:         userId,
		Username:       snapshot.Username,
		DisplayName:    snapshot.DisplayName,
		Quota:          snapshot.Quota,
		UsedQuota:      snapshot.UsedQuota,
		Balance:        openQuotaAmount(snapshot.Quota),
		Used:           openQuotaAmount(snapshot.UsedQuota),
		DisplayType:    operation_setting.GetQuotaDisplayType(),
		CurrencySymbol: operation_setting.GetCurrencySymbol(),
		RequestCount:   snapshot.RequestCount,
	})
}

// OpenRevokeCredential lets a partner drop the credential it holds, so signing
// out of their site can also end the authorization on our side.
func OpenRevokeCredential(c *gin.Context) {
	raw := c.GetHeader("Authorization")
	if idx := strings.LastIndex(raw, " "); idx >= 0 {
		raw = raw[idx+1:]
	}
	if err := model.RevokeOpenCredentialByToken(strings.TrimSpace(raw)); err != nil {
		if errors.Is(err, model.ErrOpenCredentialInvalid) {
			middleware.AbortOpenApi(c, http.StatusUnauthorized, middleware.OpenErrCredentialInvalid,
				"The credential is invalid.")
			return
		}
		common.SysLog("open credential revoke error: " + err.Error())
		middleware.AbortOpenApi(c, http.StatusInternalServerError, middleware.OpenErrInternal,
			"Internal error while revoking the credential.")
		return
	}
	common.ApiSuccess(c, gin.H{"revoked": true})
}
