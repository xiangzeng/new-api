package service

import (
	"math"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// 渠道用户消耗排名的「倍率」列：按当前定价配置算出每个 用户 × 分组 的生效倍率，
// 让千人千面折扣在排名里一眼可见。取值顺序与计费路径 relay/helper/price.go 的
// HandleGroupRatio 完全一致：千人千面 > 用户组特殊倍率 > 分组默认倍率，再乘站长折扣。
// 注意这是「现在」的配置值，不是请求当时实际扣费用的倍率。

const (
	ChannelUserRatioSourceCustom    = "custom"
	ChannelUserRatioSourceUserGroup = "user_group"
	ChannelUserRatioSourceDefault   = "default"
)

// resolveUserGroupRatio 解析用户在某分组的折扣前倍率与来源
func resolveUserGroupRatio(pricing dto.UserCustomPricing, userGroup, usingGroup string) (float64, string) {
	if pricing.Enabled {
		if gp, ok := pricing.Groups[usingGroup]; ok {
			return gp.Ratio, ChannelUserRatioSourceCustom
		}
	}
	if ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, usingGroup); ok {
		return ratio, ChannelUserRatioSourceUserGroup
	}
	return ratio_setting.GetGroupRatio(usingGroup), ChannelUserRatioSourceDefault
}

// FillChannelUserUsageRatios 为排名行的每个 用户 × 分组 填入当前生效倍率（含站长折扣）。
// 用户已删除或站长绑定查询失败时该行保持零值，不阻断整体返回。
func FillChannelUserUsageRatios(rows []*model.ChannelUserUsage, now int64) {
	for _, row := range rows {
		user, err := model.GetUserCache(row.UserID)
		if err != nil {
			continue
		}
		pricing := user.GetCustomPricing()
		for _, item := range row.Groups {
			base, source := resolveUserGroupRatio(pricing, user.Group, item.Group)
			item.BaseRatio = base
			item.RatioSource = source
			item.DefaultRatio = ratio_setting.GetGroupRatio(item.Group)
			item.Ratio = base

			reseller, err := model.ResolveActiveResellerPricing(row.UserID, item.Group, now)
			if err != nil || reseller == nil {
				continue
			}
			item.ResellerMultiplier = float64(reseller.MultiplierBps) / model.ResellerMultiplierBaseBps
			item.Ratio = math.Round(base*item.ResellerMultiplier*1e6) / 1e6
		}
	}
}
