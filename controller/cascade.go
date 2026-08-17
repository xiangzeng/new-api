package controller

import (
	"net/http"
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
}

type cascadeGroupItem struct {
	Name string `json:"name"`
	// Orphan 表示该分组已从「分组倍率」配置中删除（用户不可选），但渠道 group
	// 字段上仍残留组名。编排页据此打「已失效」标记并提供一键清理。
	Orphan   bool                 `json:"orphan"`
	Channels []cascadeChannelItem `json:"channels"`
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

	groupMap := make(map[string][]cascadeChannelItem)
	for _, channel := range channels {
		item := cascadeChannelItem{
			Id:       channel.Id,
			Name:     channel.Name,
			Type:     channel.Type,
			Status:   channel.Status,
			Priority: channel.GetPriority(),
			Weight:   channel.GetWeight(),
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
	// 失效分组沉底，避免插在中间干扰正常泳道的拖拽编排
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Orphan != groups[j].Orphan {
			return !groups[i].Orphan
		}
		return groups[i].Name < groups[j].Name
	})

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
}

// UpdateCascadeOrder 保存分组编排顺序（cascade_order 配置，与渠道优先级解耦，
// 不再改写 channel/ability priority；每组独立，跨组互不影响）
func UpdateCascadeOrder(c *gin.Context) {
	var req cascadeOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误: " + err.Error()})
		return
	}
	if len(req.Orders) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "参数错误: orders 为空"})
		return
	}

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
