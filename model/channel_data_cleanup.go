package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

// ChannelDailyUsageRetentionDays 渠道日用量保留天数，与悬浮卡最多回看的天数对齐。
const ChannelDailyUsageRetentionDays = 30

// StartChannelDataCleanupTask 每小时清理超出保留期的渠道日用量数据。
// 只在主节点跑，避免多节点重复删同一批行。
func StartChannelDataCleanupTask() {
	if !common.IsMasterNode {
		return
	}
	go func() {
		for {
			time.Sleep(time.Hour)
			CleanupOldChannelDailyUsage(ChannelDailyUsageRetentionDays)
		}
	}()
}
