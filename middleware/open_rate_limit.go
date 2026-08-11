package middleware

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

const (
	openRateLimitNamespace = "openApi:v1"
	openRateLimitWindow    = int64(60)
)

func openBalanceRateLimitKey(credentialId int64) string {
	return fmt.Sprintf("%s:balance:%d", openRateLimitNamespace, credentialId)
}

func writeOpenRateLimited(c *gin.Context, retryAfterSeconds int64, message string) {
	if retryAfterSeconds > 0 {
		c.Header("Retry-After", strconv.FormatInt(retryAfterSeconds, 10))
	}
	AbortOpenApi(c, http.StatusTooManyRequests, OpenErrRateLimited, message)
}

// openTakeRateLimit applies one fixed window slot to an arbitrary key, in Redis
// when available and in the shared in-memory limiter otherwise.
func openTakeRateLimit(c *gin.Context, key string, maxRequestNum int, subject string) bool {
	if maxRequestNum <= 0 {
		return true
	}
	if !common.RedisEnabled {
		if !inMemoryRateLimiter.Request(key, maxRequestNum, openRateLimitWindow) {
			writeOpenRateLimited(c, openRateLimitWindow, "Too many requests, please retry later.")
			return false
		}
		return true
	}
	allowed, _, ttlSeconds, err := redisFixedWindowTake(c.Request.Context(), key, maxRequestNum, openRateLimitWindow)
	if err != nil {
		abortForRateLimitCheckFailure(c, err, subject)
		return false
	}
	if !allowed {
		writeOpenRateLimited(c, ttlSeconds, "Too many requests, please retry later.")
		return false
	}
	return true
}

// OpenBalanceRateLimit throttles balance reads per key, which bounds a program
// that polls without backing off. It is deliberately not keyed by client IP:
// several keys may legitimately share one address, and one key may legitimately
// be read from several.
func OpenBalanceRateLimit() func(c *gin.Context) {
	if !common.RedisEnabled {
		inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	}
	return func(c *gin.Context) {
		credentialId := c.GetInt64(ContextKeyOpenCredentialId)
		if credentialId <= 0 {
			AbortOpenApi(c, http.StatusUnauthorized, OpenErrCredentialInvalid,
				"The credential is invalid.")
			return
		}
		limit := system_setting.GetOpenBalanceApiSettings().BalanceRateLimitPerMinute
		if !openTakeRateLimit(c, openBalanceRateLimitKey(credentialId), limit, "open-balance") {
			return
		}
		c.Next()
	}
}
