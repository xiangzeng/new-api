package service

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// 容量绕行记忆：上游报「本次请求超出该渠道输入上限」（IsChannelCapacityError）后，
// 记住「亲和键 + 渠道」一段时间，期间同一亲和键的请求直接跳过该渠道，免得每次先白跑一趟。
// 固定时长、跳过不续期：到期后回该渠道试一次，上下文已压缩就自然回归，仍超限则再记一轮。
// 只存本实例内存，重启清空（代价是多白跑一次）。详见 docs/channel/cascade-failover.md

const (
	channelAffinityCapacitySkipTTL = 10 * time.Minute

	ginKeyChannelAffinityCapacitySkips = "channel_affinity_capacity_skip_channel_ids"

	channelAffinityDetourReasonCapacity = "capacity_exceeded"
)

var (
	channelAffinityCapacitySkipMu sync.Mutex
	// 亲和缓存键 → 渠道 ID → 到期时间
	channelAffinityCapacitySkips = map[string]map[int]time.Time{}
)

// RecordChannelAffinityCapacityDetour 请求在某渠道命中容量错误、即将换渠道重试时调用：
// 记住当前亲和键在该渠道上超限；若该渠道正是归属渠道，在亲和日志信息上标记绕行原因。
// 归属保留不动，成功后 RecordChannelAffinity 照常写回归属。
func RecordChannelAffinityCapacityDetour(c *gin.Context, channelID int) {
	if c == nil || channelID <= 0 {
		return
	}
	cacheKey, _, ok := getChannelAffinityContext(c)
	if !ok {
		return
	}
	now := time.Now()
	channelAffinityCapacitySkipMu.Lock()
	pruneChannelAffinityCapacitySkipsLocked(now)
	channels := channelAffinityCapacitySkips[cacheKey]
	if channels == nil {
		channels = make(map[int]time.Time)
		channelAffinityCapacitySkips[cacheKey] = channels
	}
	channels[channelID] = now.Add(channelAffinityCapacitySkipTTL)
	channelAffinityCapacitySkipMu.Unlock()

	if getChannelAffinityHomeChannel(c) != channelID {
		return
	}
	c.Set(ginKeyChannelAffinityDetour, true)
	c.Set(ginKeyChannelAffinityDetourReason, channelAffinityDetourReasonCapacity)
	if anyInfo, ok := c.Get(ginKeyChannelAffinityLogInfo); ok {
		if info, ok := anyInfo.(map[string]interface{}); ok {
			info["detour"] = true
			info["detour_reason"] = channelAffinityDetourReasonCapacity
		}
	}
}

// LoadChannelAffinityCapacitySkips 选路前把当前亲和键仍在有效期内的容量跳过渠道写入请求上下文，
// 供归属绕行判定（IsChannelAffinityCapacitySkipped）与级联选路排除使用
func LoadChannelAffinityCapacitySkips(c *gin.Context) {
	if c == nil {
		return
	}
	cacheKey, _, ok := getChannelAffinityContext(c)
	if !ok {
		return
	}
	now := time.Now()
	skips := make(map[int]bool)
	channelAffinityCapacitySkipMu.Lock()
	for channelID, expiresAt := range channelAffinityCapacitySkips[cacheKey] {
		if now.Before(expiresAt) {
			skips[channelID] = true
		}
	}
	channelAffinityCapacitySkipMu.Unlock()
	if len(skips) > 0 {
		c.Set(ginKeyChannelAffinityCapacitySkips, skips)
	}
}

// IsChannelAffinityCapacitySkipped 本次请求是否应因容量绕行记忆跳过该渠道
func IsChannelAffinityCapacitySkipped(c *gin.Context, channelID int) bool {
	return getChannelAffinityCapacitySkips(c)[channelID]
}

func getChannelAffinityCapacitySkips(c *gin.Context) map[int]bool {
	if c == nil {
		return nil
	}
	anySkips, ok := c.Get(ginKeyChannelAffinityCapacitySkips)
	if !ok {
		return nil
	}
	skips, _ := anySkips.(map[int]bool)
	return skips
}

func pruneChannelAffinityCapacitySkipsLocked(now time.Time) {
	for cacheKey, channels := range channelAffinityCapacitySkips {
		for channelID, expiresAt := range channels {
			if !now.Before(expiresAt) {
				delete(channels, channelID)
			}
		}
		if len(channels) == 0 {
			delete(channelAffinityCapacitySkips, cacheKey)
		}
	}
}
