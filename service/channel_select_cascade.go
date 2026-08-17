package service

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// 级联模式辅助函数。详见 docs/channel/cascade-failover.md

// CascadeEnabled 级联模式总开关
func CascadeEnabled() bool {
	return operation_setting.GetCascadeSetting().Enabled
}

// GetTriedChannelIds 汇总本次请求已尝试过的渠道 ID：
// use_channel 记录选中即写入（含正在尝试的这一个），RetryParam 的请求级排除集
// 记录已失败的渠道，两者取并集，保证级联不会回头重试同一个渠道。
func GetTriedChannelIds(c *gin.Context, excluded map[int]struct{}) map[int]bool {
	tried := make(map[int]bool, len(excluded))
	for id := range excluded {
		tried[id] = true
	}
	if c == nil {
		return tried
	}
	for _, idStr := range c.GetStringSlice("use_channel") {
		if id, err := strconv.Atoi(idStr); err == nil {
			tried[id] = true
		}
	}
	return tried
}

// GetCascadeMaxRetryTimes 级联模式下重试循环的上限（= 候选渠道数 - 1，可被
// max_attempts_per_request 封顶）。auto 分组时聚合本次请求全部可用分组的候选数。
func GetCascadeMaxRetryTimes(c *gin.Context, tokenGroup string, modelName string, requestPath string) int {
	candidates := 0
	if tokenGroup == "auto" {
		userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		for _, autoGroup := range GetRequestAutoGroups(c, userGroup) {
			candidates += model.CountCascadeCandidates(autoGroup, modelName, requestPath)
		}
	} else {
		candidates = model.CountCascadeCandidates(tokenGroup, modelName, requestPath)
	}
	setting := operation_setting.GetCascadeSetting()
	if setting.MaxAttemptsPerRequest > 0 && candidates > setting.MaxAttemptsPerRequest {
		candidates = setting.MaxAttemptsPerRequest
	}
	if candidates < 1 {
		return 0
	}
	return candidates - 1
}
