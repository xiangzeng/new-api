package controller

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// CustomPricingConfiguredGroup is one group-ratio override shown on the list.
type CustomPricingConfiguredGroup struct {
	Name  string  `json:"name"`
	Ratio float64 `json:"ratio"`
}

type CustomPricingUserItem struct {
	Id               int      `json:"id"`
	Username         string   `json:"username"`
	DisplayName      string   `json:"display_name"`
	Group            string   `json:"group"`
	ConfiguredGroups int      `json:"configured_groups"`
	TotalGroups      int      `json:"total_groups"`
	MissingGroups    []string `json:"missing_groups"`
	// Groups is the set of overrides the admin has configured (name + ratio),
	// sorted by name. Unconfigured system groups are intentionally omitted.
	Groups []CustomPricingConfiguredGroup `json:"groups"`
}

func GetCustomPricingUsers(c *gin.Context) {
	users, err := model.GetAllCustomPricingUsers()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	allGroups := ratio_setting.GetGroupRatioCopy()
	totalGroups := len(allGroups)

	var result []CustomPricingUserItem
	for _, user := range users {
		pricing := dto.UserCustomPricing{}
		if user.CustomPricing != "" {
			_ = common.Unmarshal([]byte(user.CustomPricing), &pricing)
		}

		configuredCount := len(pricing.Groups)
		var missingGroups []string
		for groupName := range allGroups {
			if _, ok := pricing.Groups[groupName]; !ok {
				missingGroups = append(missingGroups, groupName)
			}
		}

		configuredGroups := make([]CustomPricingConfiguredGroup, 0, len(pricing.Groups))
		for groupName, gp := range pricing.Groups {
			configuredGroups = append(configuredGroups, CustomPricingConfiguredGroup{
				Name:  groupName,
				Ratio: gp.Ratio,
			})
		}
		// Stable order for list display.
		sort.Slice(configuredGroups, func(i, j int) bool {
			return configuredGroups[i].Name < configuredGroups[j].Name
		})

		result = append(result, CustomPricingUserItem{
			Id:               user.Id,
			Username:         user.Username,
			DisplayName:      user.DisplayName,
			Group:            user.Group,
			ConfiguredGroups: configuredCount,
			TotalGroups:      totalGroups,
			MissingGroups:    missingGroups,
			Groups:           configuredGroups,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func GetUserCustomPricing(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pricing := dto.UserCustomPricing{}
	if user.CustomPricing != "" {
		_ = common.Unmarshal([]byte(user.CustomPricing), &pricing)
	}

	allGroups := ratio_setting.GetGroupRatioCopy()

	type GroupDetail struct {
		Ratio        *float64 `json:"ratio"`
		DefaultRatio float64  `json:"default_ratio"`
		Configured   bool     `json:"configured"`
	}

	groups := make(map[string]GroupDetail)
	for groupName, defaultRatio := range allGroups {
		detail := GroupDetail{
			DefaultRatio: defaultRatio,
			Configured:   false,
		}
		if pricing.Groups != nil {
			if gp, ok := pricing.Groups[groupName]; ok {
				detail.Ratio = &gp.Ratio
				detail.Configured = true
			}
		}
		groups[groupName] = detail
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled":      pricing.Enabled,
			"groups":       groups,
			"extra_groups": pricing.ExtraGroups,
			"hide_groups":  pricing.HideGroups,
			"all_groups":   allGroups,
		},
	})
}

func UpdateUserCustomPricing(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	var req dto.UserCustomPricing
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	allGroups := ratio_setting.GetGroupRatioCopy()
	for groupName, gp := range req.Groups {
		if _, ok := allGroups[groupName]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "分组 " + groupName + " 不存在",
			})
			return
		}
		if gp.Ratio < 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "分组 " + groupName + " 的倍率不能为负数",
			})
			return
		}
	}

	// 校验 ExtraGroups 中的分组必须存在
	for groupName := range req.ExtraGroups {
		if _, ok := allGroups[groupName]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "额外可见分组 " + groupName + " 不存在",
			})
			return
		}
	}

	// 校验 HideGroups 中的分组必须存在
	for _, groupName := range req.HideGroups {
		if _, ok := allGroups[groupName]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "隐藏分组 " + groupName + " 不存在",
			})
			return
		}
	}

	pricingJSON, err := common.Marshal(req)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if err := model.UpdateUserCustomPricing(id, string(pricingJSON)); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func DeleteUserCustomPricing(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	emptyPricing := dto.UserCustomPricing{Enabled: false}
	pricingJSON, _ := common.Marshal(emptyPricing)

	if err := model.UpdateUserCustomPricing(id, string(pricingJSON)); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
