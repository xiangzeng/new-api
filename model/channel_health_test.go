package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func resetCascadeHealthForTest(t *testing.T) {
	t.Helper()
	channelHealthLock.Lock()
	channelHealthRegistry = make(map[int]*channelHealthEntry)
	channelHealthLock.Unlock()

	setting := operation_setting.GetCascadeSetting()
	orig := *setting
	t.Cleanup(func() {
		*setting = orig
		channelHealthLock.Lock()
		channelHealthRegistry = make(map[int]*channelHealthEntry)
		channelHealthLock.Unlock()
	})
	setting.Enabled = true
	setting.FailureThreshold = 1
	setting.CooldownSeconds = 120
	setting.ProbeEnabled = true
	setting.RecoverySuccessCount = 3
}

func TestChannelHealthTripAndRecover(t *testing.T) {
	resetCascadeHealthForTest(t)

	if !IsChannelHealthAvailable(1) {
		t.Fatal("channel should start healthy")
	}
	if tripped := MarkChannelHealthFailure(1, "boom"); !tripped {
		t.Fatal("first failure should trip with threshold 1")
	}
	if IsChannelHealthAvailable(1) {
		t.Fatal("tripped channel should be unavailable")
	}

	// 连续成功恢复：前两次不恢复，第三次恢复
	if MarkChannelHealthSuccess(1) {
		t.Fatal("1st success should not recover yet")
	}
	if MarkChannelHealthSuccess(1) {
		t.Fatal("2nd success should not recover yet")
	}
	if !MarkChannelHealthSuccess(1) {
		t.Fatal("3rd success should recover")
	}
	if !IsChannelHealthAvailable(1) {
		t.Fatal("recovered channel should be available")
	}
}

func TestChannelHealthFailureResetsRecoveryCount(t *testing.T) {
	resetCascadeHealthForTest(t)

	MarkChannelHealthFailure(2, "boom")
	MarkChannelHealthSuccess(2)
	MarkChannelHealthSuccess(2)
	// 失败清零重数
	MarkChannelHealthFailure(2, "boom again")
	if MarkChannelHealthSuccess(2) {
		t.Fatal("success count should have been reset by failure")
	}
}

func TestChannelHealthThreshold(t *testing.T) {
	resetCascadeHealthForTest(t)
	operation_setting.GetCascadeSetting().FailureThreshold = 3

	if MarkChannelHealthFailure(3, "e1") {
		t.Fatal("should not trip below threshold")
	}
	if MarkChannelHealthFailure(3, "e2") {
		t.Fatal("should not trip below threshold")
	}
	if !IsChannelHealthAvailable(3) {
		t.Fatal("channel below threshold should stay available")
	}
	if !MarkChannelHealthFailure(3, "e3") {
		t.Fatal("3rd failure should trip")
	}
}

func TestChannelHealthHalfOpenWhenProbeDisabled(t *testing.T) {
	resetCascadeHealthForTest(t)
	setting := operation_setting.GetCascadeSetting()
	setting.ProbeEnabled = false

	MarkChannelHealthFailure(4, "boom")
	if IsChannelHealthAvailable(4) {
		t.Fatal("should be unavailable during cooldown")
	}
	// 手动把冷却期推到过去，模拟冷却期满
	channelHealthLock.Lock()
	channelHealthRegistry[4].cooldownUntil = time.Now().Add(-time.Second)
	channelHealthLock.Unlock()
	if !IsChannelHealthAvailable(4) {
		t.Fatal("should be available after cooldown when probing disabled (half-open)")
	}
}

func TestChannelHealthSnapshotAndReset(t *testing.T) {
	resetCascadeHealthForTest(t)

	MarkChannelHealthFailure(5, "boom")
	snapshot := GetChannelHealthSnapshot()
	info, ok := snapshot[5]
	if !ok {
		t.Fatal("snapshot should contain tripped channel")
	}
	if info.State != ChannelHealthStateCooling {
		t.Fatalf("expected cooling state, got %s", info.State)
	}
	if ids := ListTrippedChannelIds(); len(ids) != 1 || ids[0] != 5 {
		t.Fatalf("expected [5], got %v", ids)
	}
	ResetChannelHealth(5)
	if !IsChannelHealthAvailable(5) {
		t.Fatal("reset channel should be available")
	}
	if len(ListTrippedChannelIds()) != 0 {
		t.Fatal("no tripped channels expected after reset")
	}
}
