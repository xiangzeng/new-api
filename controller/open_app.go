package controller

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// OpenAppRequest is the admin-facing payload for creating or updating a partner
// application. The secret is never accepted from the client: it is generated
// server side and shown exactly once.
type OpenAppRequest struct {
	Name              string `json:"name"`
	AllowedIps        string `json:"allowed_ips"`
	Status            int    `json:"status"`
	ExchangeRateLimit int    `json:"exchange_rate_limit"`
}

func GetAllOpenApps(c *gin.Context) {
	apps, err := model.ListOpenApps()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, apps)
}

func CreateOpenApp(c *gin.Context) {
	var request OpenAppRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	app, secret, err := model.CreateOpenApp(request.Name, request.AllowedIps, request.ExchangeRateLimit)
	if err != nil {
		if errors.Is(err, model.ErrOpenAppNameInvalid) {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		common.ApiError(c, err)
		return
	}
	// The clear-text secret exists only in this response; it is not recoverable
	// afterwards and can only be replaced by a reset.
	common.ApiSuccess(c, gin.H{"app": app, "app_secret": secret})
}

func UpdateOpenApp(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	var request OpenAppRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	app, err := model.UpdateOpenApp(id, request.Name, request.AllowedIps, request.Status, request.ExchangeRateLimit)
	if err != nil {
		switch {
		case errors.Is(err, model.ErrOpenAppNotFound):
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		case errors.Is(err, model.ErrOpenAppNameInvalid):
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		default:
			common.ApiError(c, err)
		}
		return
	}
	common.ApiSuccess(c, app)
}

// ResetOpenAppSecret rotates a partner's secret and revokes every credential
// issued under the previous one, so a suspected leak is contained in one step.
func ResetOpenAppSecret(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	app, secret, err := model.ResetOpenAppSecret(id)
	if err != nil {
		if errors.Is(err, model.ErrOpenAppNotFound) {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"app": app, "app_secret": secret})
}

func DeleteOpenApp(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.DeleteOpenApp(id); err != nil {
		if errors.Is(err, model.ErrOpenAppNotFound) {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
