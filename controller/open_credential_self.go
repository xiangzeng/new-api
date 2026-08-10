package controller

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetSelfOpenCredentials lists the third-party balance authorizations the
// signed-in user currently has outstanding. Long-lived credentials are only
// acceptable if their owner can see and end them, so this is part of the
// feature rather than an optional extra.
func GetSelfOpenCredentials(c *gin.Context) {
	credentials, err := model.ListOpenCredentialsByUser(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, credentials)
}

func DeleteSelfOpenCredential(c *gin.Context) {
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
