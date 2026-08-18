package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func groupNames(groups []cascadeGroupItem) []string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Name)
	}
	return names
}

func TestSortCascadeGroups(t *testing.T) {
	t.Run("未配置展示顺序时按组名升序（与旧版一致）", func(t *testing.T) {
		groups := []cascadeGroupItem{
			{Name: "kiro-高缓"},
			{Name: "default"},
			{Name: "kiro-特价"},
		}
		sortCascadeGroups(groups, map[string]int{})
		assert.Equal(t, []string{"default", "kiro-特价", "kiro-高缓"}, groupNames(groups))
	})

	t.Run("按配置顺序排，未入列的按组名升序垫底", func(t *testing.T) {
		groups := []cascadeGroupItem{
			{Name: "default"},
			{Name: "kiro-高缓"},
			{Name: "Q特价"},
			{Name: "vip"},
			{Name: "Q高缓"},
		}
		sortCascadeGroups(groups, map[string]int{"Q高缓": 1, "Q特价": 2, "kiro-高缓": 3})
		assert.Equal(t, []string{"Q高缓", "Q特价", "kiro-高缓", "default", "vip"}, groupNames(groups))
	})

	t.Run("孤儿分组一律沉底，即使排在配置列表里", func(t *testing.T) {
		groups := []cascadeGroupItem{
			{Name: "dead", Orphan: true},
			{Name: "default"},
			{Name: "Q高缓"},
		}
		sortCascadeGroups(groups, map[string]int{"dead": 1, "default": 2, "Q高缓": 3})
		assert.Equal(t, []string{"default", "Q高缓", "dead"}, groupNames(groups))
	})

	t.Run("配置里含已删除分组不影响其余顺序", func(t *testing.T) {
		groups := []cascadeGroupItem{
			{Name: "default"},
			{Name: "Q特价"},
			{Name: "Q高缓"},
		}
		sortCascadeGroups(groups, map[string]int{"已删除组": 1, "Q高缓": 2, "Q特价": 3})
		assert.Equal(t, []string{"Q高缓", "Q特价", "default"}, groupNames(groups))
	})
}

func TestNormalizeGroupSequence(t *testing.T) {
	assert.Equal(t,
		[]string{"Q高缓", "Q特价", "default"},
		normalizeGroupSequence([]string{" Q高缓 ", "", "Q特价", "Q高缓", "  ", "default"}),
	)
	assert.Equal(t, []string{}, normalizeGroupSequence(nil))
}
