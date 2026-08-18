package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetChannelRpmRegistryForTest(t *testing.T) {
	t.Helper()
	channelRpmLock.Lock()
	channelRpmRegistry = make(map[int]*channelRpmRing)
	channelRpmLock.Unlock()
	t.Cleanup(func() {
		channelRpmLock.Lock()
		channelRpmRegistry = make(map[int]*channelRpmRing)
		channelRpmLock.Unlock()
	})
}

func TestChannelRpmCountsWithinWindow(t *testing.T) {
	resetChannelRpmRegistryForTest(t)
	base := time.Unix(1_700_000_000, 0)

	for i := 0; i < 5; i++ {
		recordChannelRequestAt(1, base)
	}
	recordChannelRequestAt(1, base.Add(30*time.Second))
	recordChannelRequestAt(2, base)

	assert.Equal(t, int64(6), getChannelRpmAt(1, base.Add(59*time.Second)))
	assert.Equal(t, int64(1), getChannelRpmAt(2, base.Add(59*time.Second)))
	assert.Equal(t, int64(0), getChannelRpmAt(3, base), "无流量渠道 RPM 应为 0")
}

func TestChannelRpmDropsExpiredBuckets(t *testing.T) {
	resetChannelRpmRegistryForTest(t)
	base := time.Unix(1_700_000_000, 0)

	recordChannelRequestAt(1, base)                     // 窗口最左侧
	recordChannelRequestAt(1, base.Add(40*time.Second)) // 仍在窗口内

	// 60 秒后回看：base 那次正好滑出 (now-60, now]，只剩 1 次
	assert.Equal(t, int64(1), getChannelRpmAt(1, base.Add(60*time.Second)), "过期桶应被排除")
	// 再过 60 秒全部滑出
	assert.Equal(t, int64(0), getChannelRpmAt(1, base.Add(120*time.Second)))
}

func TestChannelRpmReusesSlotWithoutStaleCount(t *testing.T) {
	resetChannelRpmRegistryForTest(t)
	base := time.Unix(1_700_000_000, 0)

	recordChannelRequestAt(1, base)
	recordChannelRequestAt(1, base)
	// 整整 60 秒后落在同一个槽位：必须清零复用，不能把旧计数累加进来
	later := base.Add(60 * time.Second)
	recordChannelRequestAt(1, later)

	assert.Equal(t, int64(1), getChannelRpmAt(1, later), "同余槽位应清零复用")
}

func TestChannelRpmIgnoresInvalidChannelId(t *testing.T) {
	resetChannelRpmRegistryForTest(t)
	base := time.Unix(1_700_000_000, 0)

	recordChannelRequestAt(0, base)
	recordChannelRequestAt(-1, base)

	assert.Equal(t, int64(0), getChannelRpmAt(0, base))
	channelRpmLock.RLock()
	size := len(channelRpmRegistry)
	channelRpmLock.RUnlock()
	assert.Equal(t, 0, size, "非法渠道 ID 不应建环")
}

func TestChannelRpmSnapshotSkipsSilentAndPrunesIdle(t *testing.T) {
	resetChannelRpmRegistryForTest(t)
	base := time.Unix(1_700_000_000, 0)

	recordChannelRequestAt(1, base)                       // 活跃
	recordChannelRequestAt(2, base.Add(-90*time.Second))  // 窗口外但未到剔除线
	recordChannelRequestAt(3, base.Add(-400*time.Second)) // 长期无流量，应被剔除

	snapshot := getChannelRpmSnapshotAt(base)
	require.Len(t, snapshot, 1)
	assert.Equal(t, int64(1), snapshot[1])

	channelRpmLock.RLock()
	_, keep2 := channelRpmRegistry[2]
	_, keep3 := channelRpmRegistry[3]
	channelRpmLock.RUnlock()
	assert.True(t, keep2, "渠道 2 未到剔除线，不应被回收")
	assert.False(t, keep3, "渠道 3 超过剔除线，应被回收")
}
