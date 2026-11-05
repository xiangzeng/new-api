package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestResolveActiveResellerPricingCacheFollowsLocalWrites(t *testing.T) {
	db := setupResellerAdminTestDB(t)
	server := useUserCacheMiniRedis(t)
	reseller := createResellerAdminUser(t, db, "cache-reseller")
	customer := createResellerAdminUser(t, db, "cache-customer")
	const now = int64(1_700_000_000)

	pricing, err := ResolveActiveResellerPricing(customer.Id, "default", now)
	require.NoError(t, err)
	assert.Nil(t, pricing)
	assert.True(t, server.Exists(resellerPricingCacheKey(customer.Id)), "unbound result must be cached")

	// 绕过本节点写入路径直接改库，模拟另一个节点的写入：缓存期内仍是旧结果
	profile := ResellerProfile{UserId: reseller.Id, Status: ResellerStatusActive, ReceivePublicId: "cache-reseller-receive-0000000001", PricingVersion: 1}
	require.NoError(t, db.Create(&profile).Error)
	require.NoError(t, db.Create(&ResellerCustomer{ResellerId: reseller.Id, CustomerId: customer.Id, RegistrationSource: ResellerRegistrationSourceAdmin, BoundAt: now, PricingVersion: 1}).Error)
	pricing, err = ResolveActiveResellerPricing(customer.Id, "default", now)
	require.NoError(t, err)
	assert.Nil(t, pricing, "another node's write is only visible after the cache expires")

	server.FastForward(hotPathCacheTTL())
	pricing, err = ResolveActiveResellerPricing(customer.Id, "default", now)
	require.NoError(t, err)
	require.NotNil(t, pricing)
	assert.Equal(t, ResellerMultiplierBaseBps, pricing.MultiplierBps)

	// 本节点改默认规则：名下客户缓存立即失效
	_, version, err := UpdateResellerPricingRule(ResellerPricingOwnerDefault, profile.Id, "", 13000, 1, now)
	require.NoError(t, err)
	assert.EqualValues(t, 2, version)
	pricing, err = ResolveActiveResellerPricing(customer.Id, "default", now)
	require.NoError(t, err)
	require.NotNil(t, pricing)
	assert.Equal(t, 13000, pricing.MultiplierBps)
	assert.Equal(t, ResellerMultiplierSourceDefaultOverall, pricing.MultiplierSource)

	// 本节点解绑：立即生效
	require.NoError(t, AdminUnbindResellerCustomer(customer.Id))
	pricing, err = ResolveActiveResellerPricing(customer.Id, "default", now)
	require.NoError(t, err)
	assert.Nil(t, pricing)

	// 本节点绑定：立即生效
	_, err = AdminBindResellerCustomer(reseller.Id, "", customer.Id, now)
	require.NoError(t, err)
	pricing, err = ResolveActiveResellerPricing(customer.Id, "default", now)
	require.NoError(t, err)
	require.NotNil(t, pricing)
	assert.Equal(t, reseller.Id, pricing.ResellerId)
}

func TestResolveActiveResellerPricingCachedRulesStillActivateOnTime(t *testing.T) {
	db := setupResellerAdminTestDB(t)
	useUserCacheMiniRedis(t)
	reseller := createResellerAdminUser(t, db, "pending-reseller")
	customer := createResellerAdminUser(t, db, "pending-customer")
	const now = int64(1_700_000_000)

	profile := ResellerProfile{UserId: reseller.Id, Status: ResellerStatusActive, ReceivePublicId: "pending-reseller-receive-00000001", PricingVersion: 1}
	require.NoError(t, db.Create(&profile).Error)
	require.NoError(t, db.Create(&ResellerCustomer{ResellerId: reseller.Id, CustomerId: customer.Id, RegistrationSource: ResellerRegistrationSourceAdmin, BoundAt: now, PricingVersion: 1}).Error)
	require.NoError(t, db.Create(&ResellerPricingRule{
		OwnerType: ResellerPricingOwnerDefault, OwnerId: profile.Id, CurrentMultiplierBps: 12000,
		PendingMultiplierBps: 15000, PendingEffectiveAt: now + 10, Version: 1,
	}).Error)

	pricing, err := ResolveActiveResellerPricing(customer.Id, "default", now)
	require.NoError(t, err)
	require.NotNil(t, pricing)
	assert.Equal(t, 12000, pricing.MultiplierBps)

	pricing, err = ResolveActiveResellerPricing(customer.Id, "default", now+10)
	require.NoError(t, err)
	require.NotNil(t, pricing)
	assert.Equal(t, 15000, pricing.MultiplierBps, "a pending increase must take effect on time even when served from cache")
}

func TestHasActiveUserSubscriptionCachesOnlyAbsence(t *testing.T) {
	// 开通订阅的事务里会再取一次数据库时间，全局测试库只有 1 个连接会自锁，这里用独立库
	previousDB, previousLogDB := DB, LOG_DB
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&User{}, &SubscriptionPlan{}, &UserSubscription{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		_ = sqlDB.Close()
	})
	server := useUserCacheMiniRedis(t)

	user := User{Username: "sub-cache-user", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, DB.Create(&user).Error)
	plan := &SubscriptionPlan{Title: "Sub cache plan", DurationUnit: SubscriptionDurationMonth, DurationValue: 1, TotalAmount: 100, Enabled: true}
	require.NoError(t, DB.Create(plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)

	hasSub, err := HasActiveUserSubscription(user.Id)
	require.NoError(t, err)
	assert.False(t, hasSub)
	assert.True(t, server.Exists(noActiveSubscriptionCacheKey(user.Id)))

	// 另一个节点开通的订阅（直接写库）：缓存期内仍按无订阅处理，过期后看到
	require.NoError(t, DB.Create(&UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 100, StartTime: 1, EndTime: common.GetTimestamp() + 3600, Status: "active"}).Error)
	hasSub, err = HasActiveUserSubscription(user.Id)
	require.NoError(t, err)
	assert.False(t, hasSub)
	server.FastForward(hotPathCacheTTL())
	hasSub, err = HasActiveUserSubscription(user.Id)
	require.NoError(t, err)
	assert.True(t, hasSub)
	require.NoError(t, DB.Where("user_id = ?", user.Id).Delete(&UserSubscription{}).Error)
	hasSub, err = HasActiveUserSubscription(user.Id)
	require.NoError(t, err)
	assert.False(t, hasSub)

	// 本节点开通订阅（无分组升级，不走分组缓存刷新）：下一次查询立即看到
	_, err = AdminBindSubscription(user.Id, plan.Id, "test")
	require.NoError(t, err)
	assert.False(t, server.Exists(noActiveSubscriptionCacheKey(user.Id)))
	hasSub, err = HasActiveUserSubscription(user.Id)
	require.NoError(t, err)
	assert.True(t, hasSub)
	assert.False(t, server.Exists(noActiveSubscriptionCacheKey(user.Id)), "presence must never be cached")
}

func TestHotPathQueriesWithoutRedisReadDatabaseEveryTime(t *testing.T) {
	db := setupResellerAdminTestDB(t)
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	reseller := createResellerAdminUser(t, db, "nocache-reseller")
	customer := createResellerAdminUser(t, db, "nocache-customer")
	const now = int64(1_700_000_000)

	pricing, err := ResolveActiveResellerPricing(customer.Id, "default", now)
	require.NoError(t, err)
	assert.Nil(t, pricing)

	profile := ResellerProfile{UserId: reseller.Id, Status: ResellerStatusActive, ReceivePublicId: "nocache-reseller-receive-0000001", PricingVersion: 1}
	require.NoError(t, db.Create(&profile).Error)
	require.NoError(t, db.Create(&ResellerCustomer{ResellerId: reseller.Id, CustomerId: customer.Id, RegistrationSource: ResellerRegistrationSourceAdmin, BoundAt: now, PricingVersion: 1}).Error)
	pricing, err = ResolveActiveResellerPricing(customer.Id, "default", now)
	require.NoError(t, err)
	require.NotNil(t, pricing)
	assert.Equal(t, reseller.Id, pricing.ResellerId)
}
