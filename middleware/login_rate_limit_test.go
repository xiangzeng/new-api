package middleware

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useCriticalRateLimitSettings(t *testing.T, maxFailures int, duration int64) {
	t.Helper()

	previousEnable := common.CriticalRateLimitEnable
	previousNum := common.CriticalRateLimitNum
	previousDuration := common.CriticalRateLimitDuration
	common.CriticalRateLimitEnable = true
	common.CriticalRateLimitNum = maxFailures
	common.CriticalRateLimitDuration = duration
	t.Cleanup(func() {
		common.CriticalRateLimitEnable = previousEnable
		common.CriticalRateLimitNum = previousNum
		common.CriticalRateLimitDuration = previousDuration
	})
}

func newLoginTestRouter(t *testing.T) *gin.Engine {
	t.Helper()

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	limit := LoginBruteForceLimit()
	router.GET("/login/accepted", limit, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	// Login handlers answer 200 with success:false, so a rejected credential is
	// reported through the context rather than the status code.
	router.GET("/login/rejected", limit, func(c *gin.Context) {
		MarkLoginCredentialFailure(c)
		c.Status(http.StatusOK)
	})
	return router
}

func TestLoginBruteForceLimitChargesOnlyRejectedCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)
	useCriticalRateLimitSettings(t, 2, 31)

	router := newLoginTestRouter(t)
	const attackerAddr = "192.0.2.50:12345"
	key := redisIPRateLimitKey(loginFailureRateLimitMark, "192.0.2.50")

	// Signing in successfully, however often, must never spend the budget.
	for i := 0; i < 5; i++ {
		assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/login/accepted", attackerAddr).Code)
	}
	assert.False(t, redisServer.Exists(key), "a successful login must not create a failure counter")

	// Two rejections fill the window; the next attempt is refused before the
	// handler runs, and correct credentials cannot buy a way past it either.
	assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/login/rejected", attackerAddr).Code)
	assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/login/rejected", attackerAddr).Code)
	blocked := performRateLimitRequest(router, "/login/rejected", attackerAddr)
	assert.Equal(t, http.StatusTooManyRequests, blocked.Code)
	assert.Equal(t, "31", blocked.Header().Get("Retry-After"))
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/login/accepted", attackerAddr).Code)

	// The lockout stays with the address that produced the failures.
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/login/accepted", "198.51.100.50:12345").Code)

	assert.Equal(t, 31*time.Second, redisServer.TTL(key))
	assert.False(t, redisServer.Exists(redisIPRateLimitKey("CT", "192.0.2.50")), "login must not touch the shared critical bucket")
}

func TestLoginBruteForceLimitFallsBackToMemory(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedisEnabled })
	useCriticalRateLimitSettings(t, 1, 41)

	router := newLoginTestRouter(t)
	const addr = "192.0.2.51:12345"

	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/login/accepted", addr).Code)
	assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/login/rejected", addr).Code)
	blocked := performRateLimitRequest(router, "/login/accepted", addr)
	assert.Equal(t, http.StatusTooManyRequests, blocked.Code)
	assert.Equal(t, "41", blocked.Header().Get("Retry-After"))
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/login/accepted", "198.51.100.51:12345").Code)
}

func TestRefreshAuthRateLimitKeepsItsOwnBucket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer, _ := useRateLimitMiniRedis(t)

	previousEnable := common.RefreshAuthRateLimitEnable
	previousNum := common.RefreshAuthRateLimitNum
	previousDuration := common.RefreshAuthRateLimitDuration
	common.RefreshAuthRateLimitEnable = true
	common.RefreshAuthRateLimitNum = 1
	common.RefreshAuthRateLimitDuration = 43
	t.Cleanup(func() {
		common.RefreshAuthRateLimitEnable = previousEnable
		common.RefreshAuthRateLimitNum = previousNum
		common.RefreshAuthRateLimitDuration = previousDuration
	})

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/auth/refresh", RefreshAuthRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	const addr = "192.0.2.52:12345"
	assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/auth/refresh", addr).Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/auth/refresh", addr).Code)

	assert.True(t, redisServer.Exists(redisIPRateLimitKey("RF", "192.0.2.52")))
	assert.Equal(t, 43*time.Second, redisServer.TTL(redisIPRateLimitKey("RF", "192.0.2.52")))
	assert.False(t, redisServer.Exists(redisIPRateLimitKey("CT", "192.0.2.52")), "refresh must not spend the login budget")
}
