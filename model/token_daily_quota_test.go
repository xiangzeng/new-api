package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertDailyQuotaTestToken(t *testing.T, key string, limit int, usedQuota int, day int) *Token {
	t.Helper()
	token := &Token{
		UserId:          1,
		Key:             key,
		Name:            key,
		Status:          common.TokenStatusEnabled,
		ExpiredTime:     -1,
		UnlimitedQuota:  true,
		DailyQuotaLimit: limit,
		DailyUsedQuota:  usedQuota,
		DailyQuotaDay:   day,
	}
	require.NoError(t, token.Insert())
	return token
}

func reloadDailyQuotaTestToken(t *testing.T, id int) Token {
	t.Helper()
	var token Token
	require.NoError(t, DB.First(&token, id).Error)
	return token
}

func TestTokenQuotaConsumptionTracksDailyUsageAcrossDays(t *testing.T) {
	truncateTables(t)
	today := tokenQuotaDay(time.Now())
	yesterday := tokenQuotaDay(time.Now().AddDate(0, 0, -1))
	token := insertDailyQuotaTestToken(t, "daily-quota-rollover", 0, 900, yesterday)

	// 当天第一笔消耗：昨天的计数作废，从本次消耗重新起算
	require.NoError(t, decreaseTokenQuota(token.Id, 100))
	got := reloadDailyQuotaTestToken(t, token.Id)
	assert.Equal(t, today, got.DailyQuotaDay)
	assert.Equal(t, 100, got.DailyUsedQuota)
	assert.Equal(t, 100, got.UsedQuota)
	assert.Equal(t, -100, got.RemainQuota)

	// 同一天内的消耗与退还都累加到当天计数
	require.NoError(t, decreaseTokenQuota(token.Id, 50))
	require.NoError(t, increaseTokenQuota(token.Id, 30))
	got = reloadDailyQuotaTestToken(t, token.Id)
	assert.Equal(t, 120, got.DailyUsedQuota)
	assert.Equal(t, 120, got.UsedQuota)

	// 跨 0 点的退还不会让新的一天出现负数用量
	require.NoError(t, DB.Model(&Token{}).Where("id = ?", token.Id).Update("daily_quota_day", yesterday).Error)
	require.NoError(t, increaseTokenQuota(token.Id, 30))
	got = reloadDailyQuotaTestToken(t, token.Id)
	assert.Equal(t, today, got.DailyQuotaDay)
	assert.Equal(t, 0, got.DailyUsedQuota)
	assert.Equal(t, 90, got.UsedQuota)
}

func TestValidateUserTokenEnforcesDailyQuotaLimit(t *testing.T) {
	truncateTables(t)
	today := tokenQuotaDay(time.Now())
	yesterday := tokenQuotaDay(time.Now().AddDate(0, 0, -1))

	cases := []struct {
		name      string
		limit     int
		usedQuota int
		day       int
		wantErr   error
	}{
		{name: "below limit", limit: 100, usedQuota: 99, day: today, wantErr: nil},
		{name: "limit reached", limit: 100, usedQuota: 100, day: today, wantErr: ErrTokenDailyQuotaExhausted},
		{name: "yesterday usage resets", limit: 100, usedQuota: 500, day: yesterday, wantErr: nil},
		{name: "no daily limit", limit: 0, usedQuota: 500, day: today, wantErr: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := "daily-quota-validate-" + tc.name
			insertDailyQuotaTestToken(t, key, tc.limit, tc.usedQuota, tc.day)

			got, err := ValidateUserToken(key)
			require.NotNil(t, got)
			assert.ErrorIs(t, err, tc.wantErr)
			// 每日额度拦截不改令牌状态，次日自动恢复
			assert.Equal(t, common.TokenStatusEnabled, got.Status)
		})
	}
}

func TestTokenTodayUsedQuotaCombinesRedisCounterWithDatabase(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	today := tokenQuotaDay(time.Now())
	token := insertDailyQuotaTestToken(t, "daily-quota-redis", 1000, 150, today)

	// 当天计数不存在时的退还不生成负数键
	require.NoError(t, cacheIncrTokenDailyUsed(token.Id, -50))
	cached, err := cacheGetTokenDailyUsed(token.Id, today)
	require.NoError(t, err)
	assert.Equal(t, 0, cached)
	assert.Equal(t, 150, token.TodayUsedQuota())

	// Redis 实时计数领先于批量落库的数据库值
	require.NoError(t, cacheIncrTokenDailyUsed(token.Id, 200))
	assert.Equal(t, 200, token.TodayUsedQuota())

	// Redis 计数落后（如 Redis 数据丢失后重新计数）时以数据库值兜底
	token.DailyUsedQuota = 300
	assert.Equal(t, 300, token.TodayUsedQuota())
}
