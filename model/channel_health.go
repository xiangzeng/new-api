package model

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// 渠道运行时健康状态（级联熔断用）。
// 纯内存注册表：不写渠道表、不影响启用/禁用状态，进程重启后全部视为健康。
// 详见 docs/channel/cascade-failover.md

const (
	ChannelHealthStateHealthy = "healthy"
	ChannelHealthStateCooling = "cooling"
	ChannelHealthStateProbing = "probing"
)

type channelHealthEntry struct {
	consecutiveFailures int
	tripped             bool
	trippedAt           time.Time
	cooldownUntil       time.Time
	recoverySuccesses   int
	lastError           string
	lastErrorAt         time.Time
}

// ChannelHealthInfo 健康状态快照（供编排 API / 前端展示）
type ChannelHealthInfo struct {
	ChannelId           int    `json:"channel_id"`
	State               string `json:"state"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	RecoverySuccesses   int    `json:"recovery_successes"`
	TrippedAt           int64  `json:"tripped_at,omitempty"`
	CooldownRemaining   int64  `json:"cooldown_remaining,omitempty"`
	LastError           string `json:"last_error,omitempty"`
	LastErrorAt         int64  `json:"last_error_at,omitempty"`
}

var (
	channelHealthLock     sync.RWMutex
	channelHealthRegistry = make(map[int]*channelHealthEntry)
)

// MarkChannelHealthFailure 记录一次渠道故障类错误。
// 返回值表示本次调用是否触发了熔断（从可用变为不可用）。
func MarkChannelHealthFailure(channelId int, errMsg string) bool {
	if channelId <= 0 {
		return false
	}
	setting := operation_setting.GetCascadeSetting()
	now := time.Now()

	channelHealthLock.Lock()
	defer channelHealthLock.Unlock()

	entry, ok := channelHealthRegistry[channelId]
	if !ok {
		entry = &channelHealthEntry{}
		channelHealthRegistry[channelId] = entry
	}
	entry.consecutiveFailures++
	entry.recoverySuccesses = 0
	if len(errMsg) > 512 {
		errMsg = errMsg[:512]
	}
	entry.lastError = errMsg
	entry.lastErrorAt = now

	if entry.tripped {
		// 已熔断状态下再次失败：刷新冷却窗口（探活关闭时的半开兜底会用到）
		entry.cooldownUntil = now.Add(time.Duration(setting.GetCooldownSeconds()) * time.Second)
		return false
	}
	if entry.consecutiveFailures >= setting.GetFailureThreshold() {
		entry.tripped = true
		entry.trippedAt = now
		entry.cooldownUntil = now.Add(time.Duration(setting.GetCooldownSeconds()) * time.Second)
		return true
	}
	return false
}

// MarkChannelHealthSuccess 记录一次成功（真实请求或探活均可）。
// 熔断中的渠道连续成功达到恢复门槛后回到健康状态；健康渠道则清零失败计数。
// 返回值表示本次调用是否完成了恢复（从不可用变为可用）。
func MarkChannelHealthSuccess(channelId int) bool {
	if channelId <= 0 {
		return false
	}
	setting := operation_setting.GetCascadeSetting()

	channelHealthLock.Lock()
	defer channelHealthLock.Unlock()

	entry, ok := channelHealthRegistry[channelId]
	if !ok {
		return false
	}
	if !entry.tripped {
		delete(channelHealthRegistry, channelId)
		return false
	}
	entry.recoverySuccesses++
	if entry.recoverySuccesses >= setting.GetRecoverySuccessCount() {
		delete(channelHealthRegistry, channelId)
		return true
	}
	return false
}

// IsChannelHealthAvailable 判断渠道当前是否可被级联选择器选中。
// 健康 → 可用；熔断中 → 不可用；探活关闭且冷却期已过 → 半开放行。
func IsChannelHealthAvailable(channelId int) bool {
	setting := operation_setting.GetCascadeSetting()

	channelHealthLock.RLock()
	defer channelHealthLock.RUnlock()

	entry, ok := channelHealthRegistry[channelId]
	if !ok || !entry.tripped {
		return true
	}
	if !setting.ProbeEnabled && time.Now().After(entry.cooldownUntil) {
		return true
	}
	return false
}

// ListTrippedChannelIds 返回当前处于熔断状态的渠道 ID（探活循环用）
func ListTrippedChannelIds() []int {
	channelHealthLock.RLock()
	defer channelHealthLock.RUnlock()

	ids := make([]int, 0, len(channelHealthRegistry))
	for id, entry := range channelHealthRegistry {
		if entry.tripped {
			ids = append(ids, id)
		}
	}
	return ids
}

// ResetChannelHealth 清除指定渠道的健康标记（渠道被删除/手动操作时使用）。
// 返回该渠道原先是否处于熔断状态（手动恢复计数用）。
func ResetChannelHealth(channelId int) bool {
	channelHealthLock.Lock()
	defer channelHealthLock.Unlock()
	entry, ok := channelHealthRegistry[channelId]
	delete(channelHealthRegistry, channelId)
	return ok && entry.tripped
}

// GetChannelHealthSnapshot 返回全部有记录渠道的健康快照
func GetChannelHealthSnapshot() map[int]ChannelHealthInfo {
	channelHealthLock.RLock()
	defer channelHealthLock.RUnlock()

	now := time.Now()
	snapshot := make(map[int]ChannelHealthInfo, len(channelHealthRegistry))
	for id, entry := range channelHealthRegistry {
		info := ChannelHealthInfo{
			ChannelId:           id,
			State:               ChannelHealthStateHealthy,
			ConsecutiveFailures: entry.consecutiveFailures,
			RecoverySuccesses:   entry.recoverySuccesses,
			LastError:           entry.lastError,
		}
		if !entry.lastErrorAt.IsZero() {
			info.LastErrorAt = entry.lastErrorAt.Unix()
		}
		if entry.tripped {
			info.State = ChannelHealthStateProbing
			if now.Before(entry.cooldownUntil) {
				info.State = ChannelHealthStateCooling
				info.CooldownRemaining = int64(entry.cooldownUntil.Sub(now).Seconds())
			}
			info.TrippedAt = entry.trippedAt.Unix()
		}
		snapshot[id] = info
	}
	return snapshot
}
