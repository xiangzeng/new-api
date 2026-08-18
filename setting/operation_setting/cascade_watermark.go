package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// CascadeWatermarkSetting 渠道 RPM 水位线：渠道 ID → 60 秒滚动窗口内的请求数上限。
// 达到水位线的渠道在级联选择时被跳过（不管健康与否），流量自然溢出到下一个渠道。
// 0 / 未配置 = 不限流，该渠道永不视为打满——保证未配置时行为与旧版完全一致。
// 与 cascade_order 同构：编排相关配置集中在 option，不落渠道表。
// 详见 docs/channel/cascade-failover.md
type CascadeWatermarkSetting struct {
	ChannelRpm map[int]int `json:"channel_rpm"`
}

var cascadeWatermarkSetting = CascadeWatermarkSetting{
	ChannelRpm: map[int]int{},
}

func init() {
	config.GlobalConfig.Register("cascade_watermark", &cascadeWatermarkSetting)
}

func GetCascadeWatermarkSetting() *CascadeWatermarkSetting {
	return &cascadeWatermarkSetting
}

// GetChannelRpmWatermark 返回渠道的 RPM 水位线；<= 0 表示不限流
func GetChannelRpmWatermark(channelId int) int {
	watermark, ok := cascadeWatermarkSetting.ChannelRpm[channelId]
	if !ok || watermark < 0 {
		return 0
	}
	return watermark
}
