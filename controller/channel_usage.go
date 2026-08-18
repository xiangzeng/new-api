package controller

import (
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// 渠道日用量 API：渠道列表与编排页的「最近 N 天消耗」悬浮卡数据源。

// GetChannelDailyUsage 返回单个渠道在 [start_date, end_date] 区间内的分日用量，日期倒序。
func GetChannelDailyUsage(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelId <= 0 {
		common.ApiErrorMsg(c, "无效的渠道 ID")
		return
	}
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	if startDate == "" || endDate == "" {
		common.ApiErrorMsg(c, "请提供 start_date 和 end_date 参数")
		return
	}
	// 日期是直接进 SQL 比较的字符串，格式必须先校验，顺带挡住区间写反的调用
	if _, err := time.Parse(model.ChannelDailyUsageDateLayout, startDate); err != nil {
		common.ApiErrorMsg(c, "start_date 格式应为 YYYY-MM-DD")
		return
	}
	if _, err := time.Parse(model.ChannelDailyUsageDateLayout, endDate); err != nil {
		common.ApiErrorMsg(c, "end_date 格式应为 YYYY-MM-DD")
		return
	}
	if startDate > endDate {
		common.ApiErrorMsg(c, "start_date 不能晚于 end_date")
		return
	}

	usages, err := model.GetChannelDailyUsageByDateRange(channelId, startDate, endDate)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, usages)
}

// BackfillChannelDailyUsage 用使用日志重算最近 days 天的渠道日用量，供上线后补历史。
// 幂等，可重复执行。
func BackfillChannelDailyUsage(c *gin.Context) {
	var req struct {
		Days int `json:"days"`
	}
	// 允许空请求体，此时按默认天数回填
	_ = c.ShouldBindJSON(&req)

	result, err := model.BackfillChannelDailyUsage(req.Days)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}
