package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// CascadeWatermarkSetting 渠道 RPM 水位线：分组 → 渠道 ID → 该分组下 60 秒滚动窗口内的请求数上限。
// 同一渠道挂在多个分组时各组独立设置、独立计数（GroupChannelRpm）；达到水位线的渠道在该分组的
// 级联选择时被跳过（不管健康与否），流量自然溢出到下一个渠道，不影响其他分组。
// 0 / 未配置 = 不限流，该渠道永不视为打满——保证未配置时行为与旧版完全一致。
// 与 cascade_order 同构：编排相关配置集中在 option，不落渠道表。
//
// ChannelRpm 是按渠道共享的旧版水位线，只读兼容：分组未单独设置时沿用它（各组各算各的）；
// 编排 API 首次改动某渠道时把它落到该渠道所在的每个分组并删掉，此后完全按分组生效。
//
// ChannelDailyQuota 渠道每日额度上限：渠道 ID → 当天（服务器本地时间 0 点起）累计消耗上限，
// 单位为内部 quota（与消费日志 quota、渠道「已使用」同口径）。满额后级联选择器彻底不再选它
// （含熔断兜底轮），次日自动恢复。与 RPM 水位线二选一（由编排 API 保证），0 / 未配置 = 不限。
// 详见 docs/channel/cascade-failover.md
type CascadeWatermarkSetting struct {
	ChannelRpm        map[int]int            `json:"channel_rpm"`
	GroupChannelRpm   map[string]map[int]int `json:"group_channel_rpm"`
	ChannelDailyQuota map[int]int64          `json:"channel_daily_quota"`
}

var cascadeWatermarkSetting = CascadeWatermarkSetting{
	ChannelRpm:        map[int]int{},
	GroupChannelRpm:   map[string]map[int]int{},
	ChannelDailyQuota: map[int]int64{},
}

func init() {
	config.GlobalConfig.Register("cascade_watermark", &cascadeWatermarkSetting)
}

func GetCascadeWatermarkSetting() *CascadeWatermarkSetting {
	return &cascadeWatermarkSetting
}

// GetGroupChannelRpmWatermark 返回渠道在指定分组下的 RPM 水位线；<= 0 表示不限流。
// 分组未单独设置时沿用旧版渠道级水位线（见 CascadeWatermarkSetting 注释）。
func GetGroupChannelRpmWatermark(group string, channelId int) int {
	if watermark, ok := cascadeWatermarkSetting.GroupChannelRpm[group][channelId]; ok {
		return max(watermark, 0)
	}
	watermark, ok := cascadeWatermarkSetting.ChannelRpm[channelId]
	if !ok || watermark < 0 {
		return 0
	}
	return watermark
}

// GetChannelDailyQuotaCap 返回渠道的每日额度上限（quota 单位）；<= 0 表示不限
func GetChannelDailyQuotaCap(channelId int) int64 {
	quotaCap, ok := cascadeWatermarkSetting.ChannelDailyQuota[channelId]
	if !ok || quotaCap < 0 {
		return 0
	}
	return quotaCap
}
