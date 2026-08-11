package middleware

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

const (
	// ContextKeyOpenCredentialId holds the row id of the presented balance key.
	ContextKeyOpenCredentialId = "open_credential_id"

	// openCredentialTouchInterval throttles last-used bookkeeping so a program
	// that polls does not turn every balance read into a database write.
	openCredentialTouchInterval = 60
)

// Balance API error codes. These are part of the published contract: callers
// branch on the code, so the strings must stay stable even if the
// human-readable message changes.
const (
	OpenErrCredentialInvalid = "CREDENTIAL_INVALID"
	OpenErrCredentialRevoked = "CREDENTIAL_REVOKED"
	OpenErrUserDisabled      = "USER_DISABLED"
	OpenErrRateLimited       = "RATE_LIMITED"
	OpenErrInternal          = "INTERNAL_ERROR"
)

// AbortOpenApi ends a balance API request with the machine-readable envelope the
// integration contract documents. Messages are intentionally fixed English
// rather than i18n: a caller's program parses them, and locale-varying text
// would make their error handling non-deterministic.
func AbortOpenApi(c *gin.Context, status int, code string, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"success": false,
		"code":    code,
		"message": message,
	})
}

// OpenCredentialAuth authenticates the read-only balance key a user issued to
// themselves. It resolves the owning user into the standard "id" context key so
// downstream handlers read like any other authenticated route.
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
					"The credential has been revoked. Issue a new balance key.")
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
		c.Set(ContextKeyOpenCredentialId, credential.Id)

		if common.GetTimestamp()-credential.LastUsedTime >= openCredentialTouchInterval {
			credentialId := credential.Id
			gopool.Go(func() {
				model.TouchOpenCredentialLastUsed(credentialId)
			})
		}
		c.Next()
	}
}
