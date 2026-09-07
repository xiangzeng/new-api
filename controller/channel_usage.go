package controller

import (
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// 渠道日用量 API：渠道列表与编排页的「最近 N 天消耗」悬浮卡数据源，以及按用户拆分的消耗排名。

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

const (
	// channelUserUsageMaxRangeSeconds 用户排名查询的最大时间跨度（90 天），与数据看板保留周期同量级
	channelUserUsageMaxRangeSeconds = 90 * 24 * 3600
	// channelUserUsageMaxRows 排名最多返回的用户数，弹窗只看头部，长尾无意义
	channelUserUsageMaxRows = 200
)

// GetChannelUserUsage 返回渠道在 [start_timestamp, end_timestamp]（unix 秒）内按用户汇总的消耗排名，
// 回答「这条渠道的消耗是被哪些用户用掉的」。口径为数据看板小时桶，并按当前定价配置附上
// 每个 用户 × 分组 的生效倍率（千人千面可见）。
func GetChannelUserUsage(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelId <= 0 {
		common.ApiErrorMsg(c, "无效的渠道 ID")
		return
	}
	startTimestamp, endTimestamp, ok := parseFlowQuotaTimeRange(c)
	if !ok {
		return
	}
	if endTimestamp-startTimestamp > channelUserUsageMaxRangeSeconds {
		common.ApiErrorMsg(c, "时间跨度不能超过 90 天")
		return
	}

	rows, err := model.GetChannelUserUsageRanking(channelId, startTimestamp, endTimestamp, channelUserUsageMaxRows)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	service.FillChannelUserUsageRatios(rows, time.Now().Unix())
	common.ApiSuccess(c, rows)
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
