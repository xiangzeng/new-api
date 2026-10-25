package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 归属亲和硬顶判定：水位线 × 倍数。详见 docs/channel/cascade-failover.md §2.4

func TestIsChannelOverWatermarkRatio(t *testing.T) {
	resetChannelRpmRegistryForTest(t)
	setCascadeWatermarkForTest(t, true, map[int]int{34: 10})

	fillChannelRpmForTest(34, 12) // 已超水位线 10，但未达 2 倍硬顶 20

	assert.False(t, IsChannelOverWatermarkRatio(34, 0), "ratio=0 表示不设硬顶，应恒为 false")
	assert.False(t, IsChannelOverWatermarkRatio(34, -1), "负数倍数视为不设硬顶")
	assert.True(t, IsChannelOverWatermark(34), "普通水位线判定：12 >= 10 应为 true")
	assert.False(t, IsChannelOverWatermarkRatio(34, 2), "12 < 20 不应达硬顶")
	assert.True(t, IsChannelOverWatermarkRatio(34, 1.2), "12 >= ceil(10*1.2)=12 应达硬顶")

	fillChannelRpmForTest(34, 8) // 20
	assert.True(t, IsChannelOverWatermarkRatio(34, 2), "20 >= 20 应达硬顶")
}

func TestIsChannelOverWatermarkRatioIgnoresUnconfiguredOrDisabled(t *testing.T) {
	resetChannelRpmRegistryForTest(t)
	fillChannelRpmForTest(35, 100)

	// 未配置水位线的渠道永不达硬顶
	setCascadeWatermarkForTest(t, true, map[int]int{})
	assert.False(t, IsChannelOverWatermarkRatio(35, 1), "未配置水位线不应达硬顶")

	// 水位线总开关关闭时同样不参与
	setCascadeWatermarkForTest(t, false, map[int]int{35: 1})
	assert.False(t, IsChannelOverWatermarkRatio(35, 1), "水位线总开关关闭时不应达硬顶")
}

func TestIsChannelOverWatermarkRatioCeilsFractionalLimit(t *testing.T) {
	resetChannelRpmRegistryForTest(t)
	setCascadeWatermarkForTest(t, true, map[int]int{36: 15})

	fillChannelRpmForTest(36, 22) // 15*1.5 = 22.5 → 上限取 23
	assert.False(t, IsChannelOverWatermarkRatio(36, 1.5), "22 < ceil(22.5)=23 不应达硬顶")
	fillChannelRpmForTest(36, 1)
	assert.True(t, IsChannelOverWatermarkRatio(36, 1.5), "23 >= 23 应达硬顶")
}
