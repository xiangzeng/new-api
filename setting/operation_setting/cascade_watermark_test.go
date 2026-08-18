package operation_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 水位线是本仓第一个整数键 map 配置：锁死「option JSON → map[int]int」的往返，
// 以及「整体替换而非合并」（删掉的渠道不能残留旧水位线）。
func TestCascadeWatermarkConfigRoundTrip(t *testing.T) {
	setting := GetCascadeWatermarkSetting()
	orig := setting.ChannelRpm
	t.Cleanup(func() { setting.ChannelRpm = orig })

	require.NoError(t, config.UpdateConfigFromMap(setting, map[string]string{
		"channel_rpm": `{"51":220,"49":8}`,
	}))
	assert.Equal(t, 220, GetChannelRpmWatermark(51))
	assert.Equal(t, 8, GetChannelRpmWatermark(49))
	assert.Equal(t, 0, GetChannelRpmWatermark(999), "未配置的渠道 = 不限流")

	// 整体替换：不在新 JSON 里的渠道必须被清掉，不能残留旧值
	require.NoError(t, config.UpdateConfigFromMap(setting, map[string]string{
		"channel_rpm": `{"51":100}`,
	}))
	assert.Equal(t, 100, GetChannelRpmWatermark(51))
	assert.Equal(t, 0, GetChannelRpmWatermark(49), "被移除的渠道不应残留旧水位线")
}

func TestGetChannelRpmWatermarkNegativeMeansUnlimited(t *testing.T) {
	setting := GetCascadeWatermarkSetting()
	orig := setting.ChannelRpm
	t.Cleanup(func() { setting.ChannelRpm = orig })

	setting.ChannelRpm = map[int]int{7: -5}
	assert.Equal(t, 0, GetChannelRpmWatermark(7), "负数水位线按不限流处理")
}
