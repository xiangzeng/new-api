package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 倍率取值顺序必须与计费路径一致：千人千面 > 用户组特殊倍率 > 分组默认倍率；
// 千人千面未启用或未配置该分组时不得生效。
func TestResolveUserGroupRatioPriority(t *testing.T) {
	groupRatioBackup := ratio_setting.GroupRatio2JSONString()
	groupGroupRatioBackup := ratio_setting.GroupGroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"svip":2,"default":1}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"vip":{"svip":1.5}}`))
	t.Cleanup(func() {
		_ = ratio_setting.UpdateGroupRatioByJSONString(groupRatioBackup)
		_ = ratio_setting.UpdateGroupGroupRatioByJSONString(groupGroupRatioBackup)
	})

	customEnabled := dto.UserCustomPricing{
		Enabled: true,
		Groups:  map[string]dto.UserGroupPricing{"svip": {Ratio: 0.8}},
	}
	customDisabled := dto.UserCustomPricing{
		Enabled: false,
		Groups:  map[string]dto.UserGroupPricing{"svip": {Ratio: 0.8}},
	}

	tests := []struct {
		name       string
		pricing    dto.UserCustomPricing
		userGroup  string
		usingGroup string
		wantRatio  float64
		wantSource string
	}{
		{"custom pricing wins over user-group special ratio", customEnabled, "vip", "svip", 0.8, ChannelUserRatioSourceCustom},
		{"disabled custom pricing falls back to user-group special ratio", customDisabled, "vip", "svip", 1.5, ChannelUserRatioSourceUserGroup},
		{"custom pricing without this group falls back to user-group special ratio", customEnabled, "vip", "default", 1, ChannelUserRatioSourceDefault},
		{"no special ratio uses group default", customDisabled, "default", "svip", 2, ChannelUserRatioSourceDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratio, source := resolveUserGroupRatio(tt.pricing, tt.userGroup, tt.usingGroup)
			assert.Equal(t, tt.wantRatio, ratio)
			assert.Equal(t, tt.wantSource, source)
		})
	}
}
