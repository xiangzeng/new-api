package service

import (
	"sync"
	"time"
)

const DefaultCooldownSeconds = 60

var (
	cooldownMap   = make(map[int]int64)
	cooldownMutex sync.Mutex
)

func CooldownChannel(channelID int) {
	cooldownMutex.Lock()
	defer cooldownMutex.Unlock()
	cooldownMap[channelID] = time.Now().Unix() + DefaultCooldownSeconds
}

func GetCooledDownChannelIDs() map[int]struct{} {
	cooldownMutex.Lock()
	defer cooldownMutex.Unlock()
	now := time.Now().Unix()
	result := make(map[int]struct{})
	for id, expiry := range cooldownMap {
		if now < expiry {
			result[id] = struct{}{}
		} else {
			delete(cooldownMap, id)
		}
	}
	return result
}

func ClearChannelCooldown(channelID int) {
	cooldownMutex.Lock()
	defer cooldownMutex.Unlock()
	delete(cooldownMap, channelID)
}
