package model

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/go-redis/redis/v8"
)

func cacheSetToken(token Token) error {
	key := common.GenerateHMAC(token.Key)
	token.Clean()
	err := common.RedisHSetObj(fmt.Sprintf("token:%s", key), &token, time.Duration(common.RedisKeyCacheSeconds())*time.Second)
	if err != nil {
		return err
	}
	return nil
}

func cacheDeleteToken(key string) error {
	key = common.GenerateHMAC(key)
	err := common.RedisDelKey(fmt.Sprintf("token:%s", key))
	if err != nil {
		return err
	}
	return nil
}

func cacheIncrTokenQuota(key string, increment int64) error {
	key = common.GenerateHMAC(key)
	err := common.RedisHIncrBy(fmt.Sprintf("token:%s", key), constant.TokenFiledRemainQuota, increment)
	if err != nil {
		return err
	}
	return nil
}

func cacheDecrTokenQuota(key string, decrement int64) error {
	return cacheIncrTokenQuota(key, -decrement)
}

func cacheSetTokenField(key string, field string, value string) error {
	key = common.GenerateHMAC(key)
	err := common.RedisHSetField(fmt.Sprintf("token:%s", key), field, value)
	if err != nil {
		return err
	}
	return nil
}

// CacheGetTokenByKey 从缓存中获取 token，如果缓存中不存在，则从数据库中获取
func cacheGetTokenByKey(key string) (*Token, error) {
	hmacKey := common.GenerateHMAC(key)
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}
	var token Token
	err := common.RedisHGetObj(fmt.Sprintf("token:%s", hmacKey), &token)
	if err != nil {
		return nil, err
	}
	token.Key = key
	return &token, nil
}

// 令牌当天用量计数按自然日分键，0 点后自动落到新键，旧键 48 小时后过期
const tokenDailyUsedCacheTTL = 48 * time.Hour

// 退还（负增量）只在当天计数已存在时扣回，避免跨 0 点的退款在新的一天凭空生成负数计数
const tokenDailyUsedIncrScript = `
local delta = tonumber(ARGV[1])
if delta < 0 and redis.call('EXISTS', KEYS[1]) == 0 then
	return 0
end
local used = redis.call('INCRBY', KEYS[1], delta)
redis.call('EXPIRE', KEYS[1], ARGV[2])
return used
`

func tokenDailyUsedCacheKey(tokenId int, day int) string {
	return fmt.Sprintf("token_daily_used:%d:%d", tokenId, day)
}

func cacheIncrTokenDailyUsed(tokenId int, delta int) error {
	if delta == 0 {
		return nil
	}
	key := tokenDailyUsedCacheKey(tokenId, tokenQuotaDay(time.Now()))
	return common.RDB.Eval(context.Background(), tokenDailyUsedIncrScript, []string{key},
		delta, int(tokenDailyUsedCacheTTL.Seconds())).Err()
}

func cacheGetTokenDailyUsed(tokenId int, day int) (int, error) {
	used, err := common.RDB.Get(context.Background(), tokenDailyUsedCacheKey(tokenId, day)).Int()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return used, err
}
