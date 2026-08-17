package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func makePurgeTestChannel(t *testing.T, id int, name string, group string, models string) *Channel {
	t.Helper()
	priority := int64(0)
	weight := uint(0)
	channel := &Channel{
		Id:       id,
		Name:     name,
		Group:    group,
		Models:   models,
		Status:   common.ChannelStatusEnabled,
		Priority: &priority,
		Weight:   &weight,
	}
	if err := DB.Create(channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	if err := channel.AddAbilities(nil); err != nil {
		t.Fatalf("add abilities: %v", err)
	}
	return channel
}

func abilityGroups(t *testing.T, channelId int) []string {
	t.Helper()
	var groups []string
	if err := DB.Model(&Ability{}).Where("channel_id = ?", channelId).
		Distinct(commonGroupCol).Pluck(commonGroupCol, &groups).Error; err != nil {
		t.Fatalf("pluck ability groups: %v", err)
	}
	return groups
}

func TestPurgeChannelGroupRemovesGroupAndRebuildsAbilities(t *testing.T) {
	truncateTables(t)
	makePurgeTestChannel(t, 901, "multi", "default,orphan", "gpt-4,gpt-5")

	result, err := PurgeChannelGroup("orphan")
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("updated = %d, want 1", result.Updated)
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("skipped = %v, want empty", result.Skipped)
	}

	var reloaded Channel
	if err := DB.First(&reloaded, 901).Error; err != nil {
		t.Fatalf("reload channel: %v", err)
	}
	if reloaded.Group != "default" {
		t.Fatalf("group = %q, want %q", reloaded.Group, "default")
	}

	groups := abilityGroups(t, 901)
	if len(groups) != 1 || groups[0] != "default" {
		t.Fatalf("ability groups = %v, want [default]", groups)
	}
}

func TestPurgeChannelGroupSkipsSoleGroupChannel(t *testing.T) {
	truncateTables(t)
	makePurgeTestChannel(t, 902, "lonely", "orphan", "gpt-4")

	result, err := PurgeChannelGroup("orphan")
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if result.Updated != 0 {
		t.Fatalf("updated = %d, want 0", result.Updated)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Id != 902 {
		t.Fatalf("skipped = %v, want channel 902", result.Skipped)
	}

	// 跳过的渠道必须原样保留，不能被摘成无分组的孤岛
	var reloaded Channel
	if err := DB.First(&reloaded, 902).Error; err != nil {
		t.Fatalf("reload channel: %v", err)
	}
	if reloaded.Group != "orphan" {
		t.Fatalf("group = %q, want %q", reloaded.Group, "orphan")
	}
	if groups := abilityGroups(t, 902); len(groups) != 1 || groups[0] != "orphan" {
		t.Fatalf("ability groups = %v, want [orphan]", groups)
	}
}

func TestPurgeChannelGroupLeavesUnrelatedChannelsUntouched(t *testing.T) {
	truncateTables(t)
	makePurgeTestChannel(t, 903, "target", "vip,orphan", "gpt-4")
	makePurgeTestChannel(t, 904, "bystander", "vip,svip", "gpt-4")

	result, err := PurgeChannelGroup("orphan")
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("updated = %d, want 1", result.Updated)
	}

	var bystander Channel
	if err := DB.First(&bystander, 904).Error; err != nil {
		t.Fatalf("reload channel: %v", err)
	}
	if bystander.Group != "vip,svip" {
		t.Fatalf("group = %q, want %q", bystander.Group, "vip,svip")
	}
}

func TestPurgeChannelGroupRejectsEmptyGroup(t *testing.T) {
	truncateTables(t)
	if _, err := PurgeChannelGroup("  "); err == nil {
		t.Fatal("expected error for empty group")
	}
}

func TestPurgeChannelGroupNoMatchIsNoop(t *testing.T) {
	truncateTables(t)
	makePurgeTestChannel(t, 905, "keep", "default", "gpt-4")

	result, err := PurgeChannelGroup("nonexistent")
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if result.Updated != 0 || len(result.Skipped) != 0 {
		t.Fatalf("result = %+v, want empty", result)
	}
	if groups := abilityGroups(t, 905); len(groups) != 1 || groups[0] != "default" {
		t.Fatalf("ability groups = %v, want [default]", groups)
	}
}
