package model

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// 计费热路径上每个请求都要做的两次读库：代理商定价、是否有生效订阅。
// 节点离数据库远时（美国节点到香港库单次往返约 166ms），这两项占了首字前的大头，
// 所以在 Redis 里缓存一个同步周期（SYNC_FREQUENCY，默认 60 秒）。
//
// 一致性边界：本节点的写操作会在事务提交后主动删除缓存；其他节点各用自己的 Redis，
// 最多晚一个 TTL 才看到变化，与渠道、倍率等配置的同步延迟一致。
// Redis 不可用或读写出错时一律直接查库，缓存故障只会变慢、不会算错。

const (
	resellerPricingCacheKeyPrefix = "reseller_pricing:v1:"
	// 只缓存「没有生效订阅」：有订阅的用户每次照常查库，行为与改动前完全一致。
	noActiveSubscriptionCacheKeyPrefix = "sub_none:v1:"
)

// resellerPricingSnapshot 是解析代理商倍率所需的全部库内数据。
// 倍率本身不缓存：每次请求用当前时间与分组现算，涨价 24 小时后生效这类定时规则照常按秒切换。
type resellerPricingSnapshot struct {
	Bound             bool                           `json:"bound"`
	ResellerId        int                            `json:"reseller_id"`
	CustomerId        int                            `json:"customer_id"`
	CustomerBindingId int64                          `json:"customer_binding_id"`
	DefaultRules      map[string]ResellerPricingRule `json:"default_rules"`
	CustomerRules     map[string]ResellerPricingRule `json:"customer_rules"`
}

func hotPathCacheEnabled() bool {
	return common.RedisEnabled && common.RDB != nil
}

func hotPathCacheTTL() time.Duration {
	seconds := common.RedisKeyCacheSeconds()
	if seconds <= 0 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

func resellerPricingCacheKey(customerId int) string {
	return fmt.Sprintf("%s%d", resellerPricingCacheKeyPrefix, customerId)
}

func noActiveSubscriptionCacheKey(userId int) string {
	return fmt.Sprintf("%s%d", noActiveSubscriptionCacheKeyPrefix, userId)
}

func getResellerPricingSnapshot(customerId int) (*resellerPricingSnapshot, error) {
	if hotPathCacheEnabled() {
		raw, err := common.RDB.Get(context.Background(), resellerPricingCacheKey(customerId)).Result()
		if err == nil {
			var cached resellerPricingSnapshot
			if unmarshalErr := common.UnmarshalJsonStr(raw, &cached); unmarshalErr == nil {
				return &cached, nil
			}
		} else if !errors.Is(err, redis.Nil) {
			common.SysError(fmt.Sprintf("reseller pricing cache read failed for user %d: %v", customerId, err))
		}
	}

	snapshot, err := loadResellerPricingSnapshot(customerId)
	if err != nil {
		return nil, err
	}
	if hotPathCacheEnabled() {
		if encoded, marshalErr := common.Marshal(snapshot); marshalErr == nil {
			if setErr := common.RDB.Set(context.Background(), resellerPricingCacheKey(customerId), encoded, hotPathCacheTTL()).Err(); setErr != nil {
				common.SysError(fmt.Sprintf("reseller pricing cache write failed for user %d: %v", customerId, setErr))
			}
		}
	}
	return snapshot, nil
}

func loadResellerPricingSnapshot(customerId int) (*resellerPricingSnapshot, error) {
	var binding ResellerCustomer
	if err := DB.Where("customer_id = ?", customerId).First(&binding).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &resellerPricingSnapshot{CustomerId: customerId}, nil
		}
		return nil, err
	}

	var profile ResellerProfile
	if err := DB.Where("user_id = ? AND status = ?", binding.ResellerId, ResellerStatusActive).First(&profile).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &resellerPricingSnapshot{CustomerId: customerId}, nil
		}
		return nil, err
	}

	defaultRules, err := GetResellerPricingRules(ResellerPricingOwnerDefault, profile.Id)
	if err != nil {
		return nil, err
	}
	customerRules, err := GetResellerPricingRules(ResellerPricingOwnerCustomer, binding.Id)
	if err != nil {
		return nil, err
	}
	return &resellerPricingSnapshot{
		Bound:             true,
		ResellerId:        binding.ResellerId,
		CustomerId:        binding.CustomerId,
		CustomerBindingId: binding.Id,
		DefaultRules:      defaultRules,
		CustomerRules:     customerRules,
	}, nil
}

func deleteHotPathCacheKeys(keys ...string) {
	if !hotPathCacheEnabled() || len(keys) == 0 {
		return
	}
	if err := common.RDB.Del(context.Background(), keys...).Err(); err != nil {
		common.SysError(fmt.Sprintf("hot path cache invalidation failed for %d keys: %v", len(keys), err))
	}
}

// InvalidateResellerPricingCache 在客户的代理商绑定关系变化（绑定、解绑）后调用。
func InvalidateResellerPricingCache(customerIds ...int) {
	keys := make([]string, 0, len(customerIds))
	for _, customerId := range customerIds {
		if customerId > 0 {
			keys = append(keys, resellerPricingCacheKey(customerId))
		}
	}
	deleteHotPathCacheKeys(keys...)
}

// invalidateResellerPricingCacheForOwner 在定价规则改动后调用：
// 客户规则只影响这一个客户，默认规则影响该代理商名下全部客户。
func invalidateResellerPricingCacheForOwner(ownerType string, ownerId int64) {
	if !hotPathCacheEnabled() || ownerId <= 0 {
		return
	}
	var customerIds []int
	var err error
	switch ownerType {
	case ResellerPricingOwnerCustomer:
		err = DB.Model(&ResellerCustomer{}).Where("id = ?", ownerId).Pluck("customer_id", &customerIds).Error
	case ResellerPricingOwnerDefault:
		err = DB.Model(&ResellerCustomer{}).
			Where("reseller_id = (?)", DB.Model(&ResellerProfile{}).Select("user_id").Where("id = ?", ownerId)).
			Pluck("customer_id", &customerIds).Error
	default:
		return
	}
	if err != nil {
		common.SysError(fmt.Sprintf("reseller pricing cache invalidation lookup failed for %s %d: %v", ownerType, ownerId, err))
		return
	}
	InvalidateResellerPricingCache(customerIds...)
}

func cachedNoActiveSubscription(userId int) bool {
	if !hotPathCacheEnabled() {
		return false
	}
	raw, err := common.RDB.Get(context.Background(), noActiveSubscriptionCacheKey(userId)).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			common.SysError(fmt.Sprintf("subscription cache read failed for user %d: %v", userId, err))
		}
		return false
	}
	return raw == "1"
}

func rememberNoActiveSubscription(userId int) {
	if !hotPathCacheEnabled() {
		return
	}
	if err := common.RDB.Set(context.Background(), noActiveSubscriptionCacheKey(userId), "1", hotPathCacheTTL()).Err(); err != nil {
		common.SysError(fmt.Sprintf("subscription cache write failed for user %d: %v", userId, err))
	}
}

// invalidateNoActiveSubscriptionCache 在用户新增订阅后调用。
// 订阅作废、删除、到期只会让「无订阅」更加成立，不需要清缓存。
func invalidateNoActiveSubscriptionCache(userId int) {
	if userId > 0 {
		deleteHotPathCacheKeys(noActiveSubscriptionCacheKey(userId))
	}
}
