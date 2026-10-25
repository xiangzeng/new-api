package controller

import (
	"errors"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// 级联编排 API。详见 docs/channel/cascade-failover.md

type cascadeChannelItem struct {
	Id       int                       `json:"id"`
	Name     string                    `json:"name"`
	Type     int                       `json:"type"`
	Status   int                       `json:"status"`
	Priority int64                     `json:"priority"`
	Weight   int                       `json:"weight"`
	Health   *model.ChannelHealthInfo  `json:"health,omitempty"`
	Metrics  *model.ChannelMetricsInfo `json:"metrics,omitempty"`
	// Rpm 近 60 秒被选中的次数（滚动窗口，选中即记账）
	Rpm int64 `json:"rpm"`
	// RpmWatermark 该渠道的 RPM 水位线，0 = 不限流
	RpmWatermark int `json:"rpm_watermark"`
	// DailyQuotaCap 该渠道的每日额度上限（quota 单位），0 = 不限；与 RPM 水位线二选一
	DailyQuotaCap int64 `json:"daily_quota_cap"`
	// TodayUsedQuota 该渠道今天（服务器本地时间 0 点起）累计消耗（quota 单位）
	TodayUsedQuota int64 `json:"today_used_quota"`
}

type cascadeGroupItem struct {
	Name string `json:"name"`
	// Orphan 表示该分组已从「分组倍率」配置中删除（用户不可选），但渠道 group
	// 字段上仍残留组名。编排页据此打「已失效」标记并提供一键清理。
	Orphan   bool                 `json:"orphan"`
	Channels []cascadeChannelItem `json:"channels"`
}

// sortCascadeGroups 排泳道：失效（孤儿）分组一律沉底，避免插在中间干扰正常泳道的拖拽编排；
// 在役分组按 cascade_order.group_sequence 的配置顺序，未入列的按组名升序垫底
// （未配置时等价于旧版的纯组名升序）。
func sortCascadeGroups(groups []cascadeGroupItem, positions map[string]int) {
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Orphan != groups[j].Orphan {
			return !groups[i].Orphan
		}
		pi, pj := positions[groups[i].Name], positions[groups[j].Name]
		if pi != pj {
			if pi == 0 || pj == 0 {
				return pi != 0
			}
			return pi < pj
		}
		return groups[i].Name < groups[j].Name
	})
}

// GetCascadeOverview 返回级联总览：分组 → 按编排顺序的渠道列表（含健康快照）+ 当前配置
func GetCascadeOverview(c *gin.Context) {
	channels, err := model.GetAllChannels(0, -1, false, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	healthSnapshot := model.GetChannelHealthSnapshot()
	metricsSnapshot := model.GetChannelMetricsSnapshot()
	rpmSnapshot := model.GetChannelRpmSnapshot()
	todayQuotaSnapshot := model.GetChannelTodayQuotaSnapshot()

	groupMap := make(map[string][]cascadeChannelItem)
	for _, channel := range channels {
		item := cascadeChannelItem{
			Id:             channel.Id,
			Name:           channel.Name,
			Type:           channel.Type,
			Status:         channel.Status,
			Priority:       channel.GetPriority(),
			Weight:         channel.GetWeight(),
			Rpm:            rpmSnapshot[channel.Id],
			RpmWatermark:   operation_setting.GetChannelRpmWatermark(channel.Id),
			DailyQuotaCap:  operation_setting.GetChannelDailyQuotaCap(channel.Id),
			TodayUsedQuota: todayQuotaSnapshot[channel.Id],
		}
		if health, ok := healthSnapshot[channel.Id]; ok {
			healthCopy := health
			item.Health = &healthCopy
		}
		if metrics, ok := metricsSnapshot[channel.Id]; ok {
			metricsCopy := metrics
			item.Metrics = &metricsCopy
		}
		for _, group := range strings.Split(channel.Group, ",") {
			group = strings.TrimSpace(group)
			if group == "" {
				continue
			}
			groupMap[group] = append(groupMap[group], item)
		}
	}

	groups := make([]cascadeGroupItem, 0, len(groupMap))
	for name, items := range groupMap {
		// 与级联选择器同规则：编排顺序在前，未入列按优先级降序、id 升序垫底
		pos := model.CascadeOrderPositions(name)
		sort.SliceStable(items, func(i, j int) bool {
			pi, pj := pos[items[i].Id], pos[items[j].Id]
			if pi != 0 || pj != 0 {
				if pi == 0 || pj == 0 {
					return pi != 0
				}
				return pi < pj
			}
			if items[i].Priority != items[j].Priority {
				return items[i].Priority > items[j].Priority
			}
			return items[i].Id < items[j].Id
		})
		groups = append(groups, cascadeGroupItem{
			Name:     name,
			Orphan:   !ratio_setting.ContainsGroupRatio(name),
			Channels: items,
		})
	}
	sortCascadeGroups(groups, operation_setting.GetCascadeGroupSequencePositions())

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"groups":  groups,
			"setting": operation_setting.GetCascadeSetting(),
		},
	})
}

type cascadeOrderRequest struct {
	Orders []struct {
		Group      string `json:"group"`
		ChannelIds []int  `json:"channel_ids"`
	} `json:"orders"`
	// GroupSequence 分组泳道展示顺序，可选；只传它表示只改排序不动组内顺序
	GroupSequence []string `json:"group_sequence"`
}

// normalizeGroupSequence 清洗分组展示顺序：去空白、丢空串、按首次出现去重
func normalizeGroupSequence(names []string) []string {
	seen := make(map[string]bool, len(names))
	cleaned := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		cleaned = append(cleaned, name)
	}
	return cleaned
}

// UpdateCascadeOrder 保存分组编排顺序（cascade_order 配置，与渠道优先级解耦，
// 不再改写 channel/ability priority；每组独立，跨组互不影响），
// 同时可选保存分组泳道的展示顺序（group_sequence，纯展示、不参与路由）
func UpdateCascadeOrder(c *gin.Context) {
	var req cascadeOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误: " + err.Error()})
		return
	}
	if len(req.Orders) == 0 && req.GroupSequence == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误: orders 与 group_sequence 均为空"})
		return
	}

	if len(req.Orders) > 0 {
		current := operation_setting.GetCascadeOrderSetting().GroupOrders
		merged := make(map[string][]int, len(current)+len(req.Orders))
		for group, ids := range current {
			merged[group] = ids
		}
		for _, order := range req.Orders {
			group := strings.TrimSpace(order.Group)
			if group == "" {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误: 分组名为空"})
				return
			}
			for _, id := range order.ChannelIds {
				if id <= 0 {
					c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误: 非法渠道 ID"})
					return
				}
			}
			merged[group] = order.ChannelIds
		}

		value, err := common.Marshal(merged)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		if err := model.UpdateOption("cascade_order.group_orders", string(value)); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
	}

	if req.GroupSequence != nil {
		value, err := common.Marshal(normalizeGroupSequence(req.GroupSequence))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		if err := model.UpdateOption("cascade_order.group_sequence", string(value)); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

type cascadeLimitItem struct {
	ChannelId int `json:"channel_id"`
	Rpm       int `json:"rpm"`
	// DailyQuota 每日额度上限（quota 单位）
	DailyQuota int64 `json:"daily_quota"`
}

type cascadeWatermarkRequest struct {
	Watermarks []cascadeLimitItem `json:"watermarks"`
}

// mergeCascadeLimits 把请求里的渠道限流设置合并进现有配置（不在请求里的渠道保持原值）。
// 每个渠道只能二选一：rpm 与 daily_quota 不能同时 > 0；提交即整体覆盖该渠道的限流方式——
// 只设 rpm 会清掉额度上限，只设 daily_quota 会清掉水位线，两者都 <= 0 = 不限。
func mergeCascadeLimits(currentRpm map[int]int, currentQuota map[int]int64, items []cascadeLimitItem) (map[int]int, map[int]int64, error) {
	rpm := make(map[int]int, len(currentRpm)+len(items))
	for channelId, value := range currentRpm {
		rpm[channelId] = value
	}
	quota := make(map[int]int64, len(currentQuota)+len(items))
	for channelId, value := range currentQuota {
		quota[channelId] = value
	}
	for _, item := range items {
		if item.ChannelId <= 0 {
			return nil, nil, errors.New("参数错误: 非法渠道 ID")
		}
		if item.Rpm > 0 && item.DailyQuota > 0 {
			return nil, nil, errors.New("参数错误: RPM 水位线与每日额度上限只能二选一")
		}
		// 清除一律删键，不留 0 值残渣
		delete(rpm, item.ChannelId)
		delete(quota, item.ChannelId)
		if item.Rpm > 0 {
			rpm[item.ChannelId] = item.Rpm
		}
		if item.DailyQuota > 0 {
			quota[item.ChannelId] = item.DailyQuota
		}
	}
	return rpm, quota, nil
}

// UpdateCascadeWatermark 保存渠道限流方式：RPM 水位线（cascade_watermark.channel_rpm）
// 或每日额度上限（cascade_watermark.channel_daily_quota），二选一，规则见 mergeCascadeLimits。
func UpdateCascadeWatermark(c *gin.Context) {
	var req cascadeWatermarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误: " + err.Error()})
		return
	}
	if len(req.Watermarks) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误: watermarks 为空"})
		return
	}

	limitSetting := operation_setting.GetCascadeWatermarkSetting()
	rpm, quota, err := mergeCascadeLimits(limitSetting.ChannelRpm, limitSetting.ChannelDailyQuota, req.Watermarks)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	rpmValue, err := common.Marshal(rpm)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	quotaValue, err := common.Marshal(quota)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	// 两项配置分两次写入，中途失败直接报错，管理员重新保存即可收敛
	if err := model.UpdateOption("cascade_watermark.channel_daily_quota", string(quotaValue)); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.UpdateOption("cascade_watermark.channel_rpm", string(rpmValue)); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

// PurgeCascadeGroup 清理孤儿分组：把已从分组配置中删除的组名从所有渠道上摘掉，
// 并同步移除该组的编排顺序。只对孤儿分组生效——在役分组一律拒绝，防止误清。
func PurgeCascadeGroup(c *gin.Context) {
	var req struct {
		Group string `json:"group"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误: " + err.Error()})
		return
	}
	group := strings.TrimSpace(req.Group)
	if group == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误: 分组名为空"})
		return
	}
	// 安全阀：仍在分组倍率配置里的分组说明还在服役，不允许从渠道上摘除
	if ratio_setting.ContainsGroupRatio(group) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "分组 " + group + " 仍在分组配置中，只能清理已失效分组",
		})
		return
	}

	result, err := model.PurgeChannelGroup(group)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 编排顺序里的同名分组一并清掉，避免配置残留死数据
	current := operation_setting.GetCascadeOrderSetting().GroupOrders
	if _, ok := current[group]; ok {
		merged := make(map[string][]int, len(current))
		for name, ids := range current {
			if name == group {
				continue
			}
			merged[name] = ids
		}
		value, marshalErr := common.Marshal(merged)
		if marshalErr != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": marshalErr.Error()})
			return
		}
		if updateErr := model.UpdateOption("cascade_order.group_orders", string(value)); updateErr != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": updateErr.Error()})
			return
		}
	}

	// 展示顺序里的同名分组也一并摘掉，别留死组名占位
	sequence := operation_setting.GetCascadeGroupSequence()
	if slices.Contains(sequence, group) {
		remaining := make([]string, 0, len(sequence))
		for _, name := range sequence {
			if name == group {
				continue
			}
			remaining = append(remaining, name)
		}
		value, marshalErr := common.Marshal(remaining)
		if marshalErr != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": marshalErr.Error()})
			return
		}
		if updateErr := model.UpdateOption("cascade_order.group_sequence", string(value)); updateErr != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": updateErr.Error()})
			return
		}
	}

	model.InitChannelCache()
	recordManageAudit(c, "channel.group_purge", map[string]interface{}{
		"group":   group,
		"count":   result.Updated,
		"skipped": len(result.Skipped),
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": result})
}

// ResetCascadeChannelHealth 手动清除指定渠道的熔断标记（编排页「立即恢复」）
func ResetCascadeChannelHealth(c *gin.Context) {
	var req struct {
		ChannelId int `json:"channel_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ChannelId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误"})
		return
	}
	if model.ResetChannelHealth(req.ChannelId) {
		// 原先确在熔断中才计一次恢复，避免重复点击刷数
		model.RecordChannelRestore(req.ChannelId)
		model.AppendChannelHealthEvent(req.ChannelId, model.ChannelHealthEventManualReset, "管理员手动恢复")
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

// GetCascadeChannelHealthEvents 返回指定渠道近 7d 健康事件时间线 + 24h/7d 熔断次数
func GetCascadeChannelHealthEvents(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Query("channel_id"))
	if err != nil || channelId <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误: 非法渠道 ID"})
		return
	}
	result, err := model.GetChannelHealthEvents(channelId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": result})
}
