package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

const (
	openRateLimitNamespace = "openApi:v1"
	openRateLimitWindow    = int64(60)
)

// openFailureReadScript reads the failure counter without touching it, so a
// successful login attempt never consumes lockout budget.
const openFailureReadScript = `
local count = tonumber(redis.call('GET', KEYS[1]) or '0')
local ttl = redis.call('TTL', KEYS[1])
if ttl < 0 then ttl = 0 end
return {count, ttl}
`

// openFailureRecordScript increments the counter and (re)arms its expiry in one
// atomic step, so concurrent failed attempts cannot leave a counter that never
// expires and locks the account out permanently.
const openFailureRecordScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('TTL', KEYS[1])
if ttl < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
  ttl = redis.call('TTL', KEYS[1])
end
return {count, ttl}
`

// openMemoryCounterStore is the Redis-less fallback for the failure lockout.
// The shared InMemoryRateLimiter cannot serve here because it fuses "read" and
// "increment" into one call, while a lockout must be readable without spending
// an attempt.
type openMemoryCounterStore struct {
	mutex   sync.Mutex
	entries map[string]*openMemoryCounter
}

type openMemoryCounter struct {
	count     int64
	expiresAt int64
}

var openFailureStore = openMemoryCounterStore{entries: make(map[string]*openMemoryCounter)}

func (s *openMemoryCounterStore) read(key string) (int64, int64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	now := common.GetTimestamp()
	entry, ok := s.entries[key]
	if !ok || entry.expiresAt <= now {
		delete(s.entries, key)
		return 0, 0
	}
	return entry.count, entry.expiresAt - now
}

func (s *openMemoryCounterStore) increment(key string, windowSeconds int64) (int64, int64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	now := common.GetTimestamp()
	// Opportunistic sweep: the lockout map is only written on failed logins, so
	// it stays small and a full pass is cheaper than a background janitor.
	for existing, entry := range s.entries {
		if entry.expiresAt <= now {
			delete(s.entries, existing)
		}
	}
	entry, ok := s.entries[key]
	if !ok {
		entry = &openMemoryCounter{expiresAt: now + windowSeconds}
		s.entries[key] = entry
	}
	entry.count++
	return entry.count, entry.expiresAt - now
}

func (s *openMemoryCounterStore) clear(key string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.entries, key)
}

func openExchangeRateLimitKey(appId string) string {
	return fmt.Sprintf("%s:exchange:%s", openRateLimitNamespace, appId)
}

func openBalanceRateLimitKey(credentialId int64) string {
	return fmt.Sprintf("%s:balance:%d", openRateLimitNamespace, credentialId)
}

func openExchangeIpRateLimitKey(clientIp string) string {
	return fmt.Sprintf("%s:exchange-ip:%s", openRateLimitNamespace, clientIp)
}

// openFailureLockKey digests the username so the lockout key cannot be mined
// for the site's user list by anyone able to enumerate Redis keys.
func openFailureLockKey(appId string, username string) string {
	return fmt.Sprintf("%s:fail:%s:%s", openRateLimitNamespace, appId, common.GenerateHMAC(username)[:32])
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

// OpenExchangeIpBackstop bounds anonymous floods before the request reaches the
// application lookup. The site-wide GlobalAPIRateLimit is deliberately not used
// here: at its default of 360 requests per 180 seconds it would throttle an
// entire partner's user base as a single client, because every partner request
// originates from that partner's own backend.
func OpenExchangeIpBackstop() func(c *gin.Context) {
	if !common.RedisEnabled {
		inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	}
	return func(c *gin.Context) {
		limit := system_setting.GetOpenBalanceApiSettings().ExchangeIpRateLimitPerMinute
		if !openTakeRateLimit(c, openExchangeIpRateLimitKey(c.ClientIP()), limit, "open-exchange-ip") {
			return
		}
		c.Next()
	}
}

// OpenExchangeRateLimit throttles credential exchanges per partner. The limit is
// deliberately not keyed by client IP: partners call from their own backend, so
// every one of their users shares a single source address.
func OpenExchangeRateLimit() func(c *gin.Context) {
	if !common.RedisEnabled {
		inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	}
	return func(c *gin.Context) {
		app := GetOpenApp(c)
		if app == nil {
			AbortOpenApi(c, http.StatusUnauthorized, OpenErrAppUnauthorized,
				"The application credentials are invalid.")
			return
		}
		limit := app.ExchangeRateLimit
		if limit <= 0 {
			limit = system_setting.GetOpenBalanceApiSettings().ExchangeRateLimitPerMinute
		}
		if !openTakeRateLimit(c, openExchangeRateLimitKey(app.AppId), limit, "open-exchange") {
			return
		}
		c.Next()
	}
}

// OpenBalanceRateLimit throttles balance reads per credential, which bounds a
// partner frontend that polls without backing off.
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

func openFailureLockWindow() int64 {
	minutes := system_setting.GetOpenBalanceApiSettings().FailureLockMinutes
	if minutes <= 0 {
		minutes = 15
	}
	return int64(minutes) * 60
}

// OpenExchangeLockState reports whether an (app, username) pair is currently
// locked out. It is called from the handler rather than a middleware because
// the username only exists once the request body has been decoded.
//
// A Redis failure returns an error and the caller rejects the request: this is
// the primary credential-stuffing control, so it fails closed.
func OpenExchangeLockState(ctx context.Context, appId string, username string) (bool, int64, error) {
	threshold := system_setting.GetOpenBalanceApiSettings().FailureLockThreshold
	if threshold <= 0 {
		return false, 0, nil
	}
	key := openFailureLockKey(appId, username)
	if !common.RedisEnabled {
		count, ttl := openFailureStore.read(key)
		return count >= int64(threshold), ttl, nil
	}
	values, err := common.RDB.Eval(ctx, openFailureReadScript, []string{key}).Slice()
	if err != nil {
		return false, 0, err
	}
	if len(values) != 2 {
		return false, 0, fmt.Errorf("unexpected Redis reply length %d for open failure lock", len(values))
	}
	count, err := redisReplyInteger(values[0])
	if err != nil {
		return false, 0, err
	}
	ttl, err := redisReplyInteger(values[1])
	if err != nil {
		return false, 0, err
	}
	return count >= int64(threshold), ttl, nil
}

// RecordOpenExchangeFailure counts one failed password attempt for the pair.
// Failures to record are logged and swallowed: losing a count is preferable to
// turning a cache blip into a failed login for a legitimate user.
func RecordOpenExchangeFailure(ctx context.Context, appId string, username string) {
	window := openFailureLockWindow()
	key := openFailureLockKey(appId, username)
	if !common.RedisEnabled {
		openFailureStore.increment(key, window)
		return
	}
	if _, err := common.RDB.Eval(ctx, openFailureRecordScript, []string{key}, window).Slice(); err != nil {
		logger.LogError(ctx, "failed to record open exchange failure: "+err.Error())
	}
}

// ClearOpenExchangeFailures resets the counter after a successful exchange so a
// user who mistypes a few times is not penalized once they get it right.
func ClearOpenExchangeFailures(ctx context.Context, appId string, username string) {
	key := openFailureLockKey(appId, username)
	if !common.RedisEnabled {
		openFailureStore.clear(key)
		return
	}
	if err := common.RedisDel(key); err != nil {
		logger.LogError(ctx, "failed to clear open exchange failures: "+err.Error())
	}
}
