package controller

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// BalanceKeyRequest carries the label the owner picked for a key. The name is
// the only thing they choose: scope and lifetime are fixed by the feature.
type BalanceKeyRequest struct {
	Name string `json:"name"`
}

// BalanceKeyCreatedResponse returns the clear-text key exactly once. It is
// stored only as an HMAC digest, so a lost key can be revoked but never read
// back.
type BalanceKeyCreatedResponse struct {
	Id          int64  `json:"id"`
	Name        string `json:"name"`
	Key         string `json:"key"`
	Scope       string `json:"scope"`
	CreatedTime int64  `json:"created_time"`
}

// GetSelfBalanceKeys lists the read-only balance keys the signed-in user holds.
// Long-lived credentials are only acceptable if their owner can see and end
// them, so this is part of the feature rather than an optional extra.
func GetSelfBalanceKeys(c *gin.Context) {
	keys, err := model.ListOpenCredentialsByUser(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, keys)
}

func CreateSelfBalanceKey(c *gin.Context) {
	var request BalanceKeyRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	key, credential, err := model.IssueOpenCredential(c.GetInt("id"), request.Name, c.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, model.ErrOpenCredentialNameTooLong):
			common.ApiErrorI18n(c, i18n.MsgBalanceKeyNameTooLong)
		case errors.Is(err, model.ErrOpenCredentialLimitReached):
			common.ApiErrorI18n(c, i18n.MsgBalanceKeyLimitReached,
				map[string]any{"Max": model.OpenCredentialMaxPerUser})
		default:
			common.ApiError(c, err)
		}
		return
	}
	common.ApiSuccess(c, BalanceKeyCreatedResponse{
		Id:          credential.Id,
		Name:        credential.Name,
		Key:         key,
		Scope:       credential.Scope,
		CreatedTime: credential.CreatedTime,
	})
}

func DeleteSelfBalanceKey(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if err := model.RevokeOpenCredentialByUser(c.GetInt("id"), id); err != nil {
		if errors.Is(err, model.ErrOpenCredentialInvalid) {
			common.ApiErrorI18n(c, i18n.MsgInvalidParams)
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
