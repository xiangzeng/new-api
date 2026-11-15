package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupChannelRpmSharedForTest(t *testing.T, enabled bool) {
	t.Helper()
	resetChannelRpmRegistryForTest(t)
	require.NoError(t, DB.AutoMigrate(&ChannelRpmReport{}))
	require.NoError(t, DB.Where("1 = 1").Delete(&ChannelRpmReport{}).Error)

	setting := operation_setting.GetCascadeSetting()
	origEnabled := setting.SharedRpmEnabled
	setting.SharedRpmEnabled = enabled

	reset := func() {
		channelRpmSharedLock.Lock()
		channelRpmSharedPeers = make(map[string]*channelRpmPeer)
		channelRpmSharedSeq = 0
		channelRpmSharedLastEmpty = false
		channelRpmSharedLastLogTime = time.Time{}
		channelRpmSharedLock.Unlock()
		channelRpmRemote.Store(nil)
		channelRpmRemotePeers.Store(0)
	}
	reset()
	t.Cleanup(func() {
		setting.SharedRpmEnabled = origEnabled
		reset()
		DB.Where("1 = 1").Delete(&ChannelRpmReport{})
	})
}

// writePeerReportForTest 模拟另一个进程的一次上报
func writePeerReportForTest(t *testing.T, nodeId string, seq int64, rpm map[string]map[int]int64) {
	t.Helper()
	payload, err := common.Marshal(rpm)
	require.NoError(t, err)
	require.NoError(t, DB.Save(&ChannelRpmReport{NodeId: nodeId, Seq: seq, Payload: string(payload)}).Error)
}

func TestSharedChannelRpmMergesPeerAfterItAdvances(t *testing.T) {
	setupChannelRpmSharedForTest(t, true)
	base := time.Unix(1_700_000_000, 0)

	recordChannelRequestAt("claude-kiro", 22, base)
	recordChannelRequestAt("claude-kiro", 22, base)
	writePeerReportForTest(t, "peer-us", 7, map[string]map[int]int64{"claude-kiro": {22: 40, 28: 3}})

	// 首次见到对方的行：可能是早已退场进程的遗留，先不计入
	syncChannelRpmShared(base)
	assert.Equal(t, int64(2), getChannelRpmAt("claude-kiro", 22, base))
	assert.Equal(t, 0, GetChannelRpmSharedPeerCount())

	// 对方 seq 推进后才计入：全站 RPM = 本进程 + 对方上报
	writePeerReportForTest(t, "peer-us", 8, map[string]map[int]int64{"claude-kiro": {22: 44, 28: 3}})
	now := base.Add(3 * time.Second)
	syncChannelRpmShared(now)
	assert.Equal(t, int64(46), getChannelRpmAt("claude-kiro", 22, now))
	assert.Equal(t, int64(3), getChannelRpmAt("claude-kiro", 28, now), "本进程无流量的渠道也要带上对方的数")
	assert.Equal(t, 1, GetChannelRpmSharedPeerCount())

	snapshot := getChannelRpmSnapshotAt(now)
	assert.Equal(t, int64(46), snapshot["claude-kiro"][22])
	assert.Equal(t, int64(3), snapshot["claude-kiro"][28])

	// 本进程上报的只能是自己的计数，不能把对方的数再报回去（否则两边互相放大）
	var own ChannelRpmReport
	require.NoError(t, DB.Where("node_id = ?", channelRpmSharedNodeId).First(&own).Error)
	var ownRpm map[string]map[int]int64
	require.NoError(t, common.UnmarshalJsonStr(own.Payload, &ownRpm))
	assert.Equal(t, map[string]map[int]int64{"claude-kiro": {22: 2}}, ownRpm)
}

func TestSharedChannelRpmDropsPeerThatStopsReporting(t *testing.T) {
	setupChannelRpmSharedForTest(t, true)
	base := time.Unix(1_700_000_000, 0)

	writePeerReportForTest(t, "peer-us", 1, map[string]map[int]int64{"codex": {45: 30}})
	syncChannelRpmShared(base)
	writePeerReportForTest(t, "peer-us", 2, map[string]map[int]int64{"codex": {45: 30}})
	syncChannelRpmShared(base.Add(3 * time.Second))
	require.Equal(t, int64(30), getChannelRpmAt("codex", 45, base.Add(3*time.Second)))

	// 对方不再上报：TTL 内仍计入，过了 TTL 退回只算本进程
	within := base.Add(3*time.Second + channelRpmSharedPeerTTL)
	syncChannelRpmShared(within)
	assert.Equal(t, int64(30), getChannelRpmAt("codex", 45, within))

	expired := within.Add(time.Second)
	syncChannelRpmShared(expired)
	assert.Equal(t, int64(0), getChannelRpmAt("codex", 45, expired))
	assert.Equal(t, 0, GetChannelRpmSharedPeerCount())

	// 长期不上报：清掉它留在库里的行
	syncChannelRpmShared(expired.Add(channelRpmSharedRowRetention))
	var count int64
	require.NoError(t, DB.Model(&ChannelRpmReport{}).Where("node_id = ?", "peer-us").Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestSharedChannelRpmDisabledIgnoresPeersAndSkipsDatabase(t *testing.T) {
	setupChannelRpmSharedForTest(t, true)
	base := time.Unix(1_700_000_000, 0)

	writePeerReportForTest(t, "peer-us", 1, map[string]map[int]int64{"codex": {45: 30}})
	syncChannelRpmShared(base)
	writePeerReportForTest(t, "peer-us", 2, map[string]map[int]int64{"codex": {45: 30}})
	syncChannelRpmShared(base.Add(3 * time.Second))
	require.Equal(t, int64(30), getChannelRpmAt("codex", 45, base.Add(3*time.Second)))

	// 关掉开关：立即退回只算本进程，且不再写库
	operation_setting.GetCascadeSetting().SharedRpmEnabled = false
	require.NoError(t, DB.Where("1 = 1").Delete(&ChannelRpmReport{}).Error)
	recordChannelRequestAt("codex", 45, base.Add(6*time.Second))
	now := base.Add(6 * time.Second)
	syncChannelRpmShared(now)
	assert.Equal(t, int64(1), getChannelRpmAt("codex", 45, now))

	var count int64
	require.NoError(t, DB.Model(&ChannelRpmReport{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestSharedChannelRpmIdleProcessStopsWriting(t *testing.T) {
	setupChannelRpmSharedForTest(t, true)
	base := time.Unix(1_700_000_000, 0)

	// 无流量的进程（蓝绿备用容器）只补报一次空快照，之后不再写库
	syncChannelRpmShared(base)
	syncChannelRpmShared(base.Add(3 * time.Second))
	syncChannelRpmShared(base.Add(6 * time.Second))

	var own ChannelRpmReport
	require.NoError(t, DB.Where("node_id = ?", channelRpmSharedNodeId).First(&own).Error)
	assert.Equal(t, int64(1), own.Seq)

	// 有流量后恢复上报
	recordChannelRequestAt("codex", 45, base.Add(9*time.Second))
	syncChannelRpmShared(base.Add(9 * time.Second))
	require.NoError(t, DB.Where("node_id = ?", channelRpmSharedNodeId).First(&own).Error)
	assert.Equal(t, int64(2), own.Seq)
}
