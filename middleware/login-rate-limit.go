package middleware

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// loginFailureRateLimitMark namespaces the counter that meters rejected
// credentials, keeping it clear of the "CT" bucket the other critical routes
// share.
const loginFailureRateLimitMark = "LF"

const loginCredentialFailureContextKey = "login_credential_failure"

// recordLoginFailureTimeout bounds the counter write that runs after the
// response has already been sent.
const recordLoginFailureTimeout = 3 * time.Second

// MarkLoginCredentialFailure tells LoginBruteForceLimit that the handler turned
// this attempt away because the credentials were wrong. Login endpoints answer
// 200 with a success:false body, so the outcome cannot be read off the status
// code and the handler has to say so itself. Only credential rejections belong
// here: a database or parameter error is not a guess at someone's password.
func MarkLoginCredentialFailure(c *gin.Context) {
	c.Set(loginCredentialFailureContextKey, true)
}

// LoginBruteForceLimit meters failed credential checks per client IP.
// CriticalRateLimit charges every attempt before the handler runs, so an
// address that legitimately signs in often - a shared office or proxy egress,
// or a dashboard whose session refresh keeps expiring and re-authenticating -
// spends the whole brute-force budget on successful logins and then locks its
// own users out with a bare 429. Counting only rejections keeps the guard
// pointed at password guessing while honest sign-ins stay free.
func LoginBruteForceLimit() func(c *gin.Context) {
	if !common.CriticalRateLimitEnable {
		return defNext
	}
	// Keep the fallback ready before requests arrive so a concurrent Redis
	// outage cannot race the in-memory limiter's first initialization.
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	return func(c *gin.Context) {
		maxFailures := common.CriticalRateLimitNum
		duration := common.CriticalRateLimitDuration
		clientIP := c.ClientIP()

		allowed, retryAfterSeconds, err := loginFailureAllowed(c.Request.Context(), clientIP, maxFailures, duration)
		if err != nil {
			abortForRateLimitCheckFailure(c, err, "mark="+loginFailureRateLimitMark)
			return
		}
		if !allowed {
			writeRateLimited(c, retryAfterSeconds)
			return
		}

		c.Next()

		if !c.GetBool(loginCredentialFailureContextKey) {
			return
		}
		recordLoginFailure(clientIP, maxFailures, duration)
	}
}

func loginFailureMemoryKey(clientIP string) string {
	return loginFailureRateLimitMark + ":" + clientIP
}

// loginFailureAllowed reads the failure counter without touching it, so a
// request only pays for the outcome it actually produces.
func loginFailureAllowed(ctx context.Context, clientIP string, maxFailures int, duration int64) (bool, int64, error) {
	if !common.RedisEnabled {
		return inMemoryRateLimiter.Allowed(loginFailureMemoryKey(clientIP), maxFailures, duration), duration, nil
	}
	if common.RDB == nil {
		return false, 0, errors.New("Redis client is not initialized")
	}
	key := redisIPRateLimitKey(loginFailureRateLimitMark, clientIP)
	count, err := common.RDB.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return true, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	if count < int64(maxFailures) {
		return true, 0, nil
	}
	ttl, err := common.RDB.TTL(ctx, key).Result()
	if err != nil {
		return false, 0, err
	}
	retryAfterSeconds := int64(ttl.Seconds())
	if retryAfterSeconds <= 0 {
		retryAfterSeconds = duration
	}
	return false, retryAfterSeconds, nil
}

// recordLoginFailure charges one rejected credential to the window. The
// response is already on the wire by now, so a client that hung up must not
// cost the counter its increment: the write runs on a detached context.
func recordLoginFailure(clientIP string, maxFailures int, duration int64) {
	if !common.RedisEnabled {
		inMemoryRateLimiter.Record(loginFailureMemoryKey(clientIP), maxFailures, duration)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), recordLoginFailureTimeout)
	defer cancel()
	if _, _, _, err := redisFixedWindowTake(ctx, redisIPRateLimitKey(loginFailureRateLimitMark, clientIP), maxFailures, duration); err != nil {
		logger.LogError(ctx, fmt.Sprintf("failed to record login failure for %s: %v", clientIP, err))
	}
}
