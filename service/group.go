package service

import (
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func GetUserUsableGroups(userGroup string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	if userGroup != "" {
		specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
		if b {
			// 处理特殊可用分组
			for specialGroup, desc := range specialSettings {
				if strings.HasPrefix(specialGroup, "-:") {
					// 移除分组
					groupToRemove := strings.TrimPrefix(specialGroup, "-:")
					delete(groupsCopy, groupToRemove)
				} else if strings.HasPrefix(specialGroup, "+:") {
					// 添加分组
					groupToAdd := strings.TrimPrefix(specialGroup, "+:")
					groupsCopy[groupToAdd] = desc
				} else {
					// 直接添加分组
					groupsCopy[specialGroup] = desc
				}
			}
		}
		// 如果userGroup不在UserUsableGroups中，返回UserUsableGroups + userGroup
		if _, ok := groupsCopy[userGroup]; !ok {
			groupsCopy[userGroup] = "用户分组"
		}
	}
	return groupsCopy
}

func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[groupName]
	return ok
}

// GetUserUsableGroupsWithCustomPricing 在基础可见集 + 用户分组级覆盖之后，
// 叠加用户个人级覆盖（千人千面的 ExtraGroups / HideGroups），优先级最高。
func GetUserUsableGroupsWithCustomPricing(userGroup string, customPricing *dto.UserCustomPricing) map[string]string {
	groupsCopy := GetUserUsableGroups(userGroup)
	if customPricing == nil || !customPricing.Enabled {
		return groupsCopy
	}
	// 添加额外可见分组
	for name, desc := range customPricing.ExtraGroups {
		groupsCopy[name] = desc
	}
	// 强制隐藏分组（用户自身分组不可被隐藏）
	for _, name := range customPricing.HideGroups {
		if name == userGroup {
			continue
		}
		delete(groupsCopy, name)
	}
	return groupsCopy
}

// GroupInUserUsableGroupsWithCustomPricing 带用户级覆盖的分组可见性判断
func GroupInUserUsableGroupsWithCustomPricing(userGroup, groupName string, customPricing *dto.UserCustomPricing) bool {
	_, ok := GetUserUsableGroupsWithCustomPricing(userGroup, customPricing)[groupName]
	return ok
}

// GetUserAutoGroup 根据用户分组获取自动分组设置
func GetUserAutoGroup(userGroup string) []string {
	groups := GetUserUsableGroups(userGroup)
	autoGroups := make([]string, 0)
	for _, group := range setting.GetAutoGroups() {
		if _, ok := groups[group]; ok {
			autoGroups = append(autoGroups, group)
		}
	}
	return autoGroups
}

// GetUserGroupRatio 获取用户使用某个分组的倍率
// userGroup 用户分组
// group 需要获取倍率的分组
func GetUserGroupRatio(userGroup, group string) float64 {
	ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, group)
	if ok {
		return ratio
	}
	return ratio_setting.GetGroupRatio(group)
}
