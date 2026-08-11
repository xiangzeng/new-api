package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

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
// endpoints so a caller's program and the dashboard never disagree about a
// balance, and additionally honors the CUSTOM display type via
// GetUsdToCurrencyRate.
func openQuotaAmount(quota int) float64 {
	amount := float64(quota)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		return amount
	}
	return amount / common.QuotaPerUnit * operation_setting.GetUsdToCurrencyRate(operation_setting.USDExchangeRate)
}

// OpenGetBalance returns the balance key owner's wallet balance.
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

// OpenRevokeCredential lets the program holding a balance key retire it with
// the key itself, so a script can clean up without a dashboard round trip.
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
