package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 用户分组限制单测：指定分组拦截、auto 跳过被封分组、全部被封取最早恢复时间、到期自动失效。

func setAutoGroupsForTest(t *testing.T, groupsJSON string, usableJSON string) {
	t.Helper()
	origAuto := setting.AutoGroups2JsonString()
	origUsable := setting.UserUsableGroups2JSONString()
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(groupsJSON))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(usableJSON))
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(origAuto)
		_ = setting.UpdateUserUsableGroupsByJSONString(origUsable)
	})
}

func newGroupBlockContextForTest(blocks ...dto.UserGroupBlock) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUserGroupBlocks, dto.UserGroupBlocks{Blocks: blocks})
	return c
}

func TestUserGroupBlocksActiveBlock(t *testing.T) {
	now := time.Now().Unix()
	blocks := dto.UserGroupBlocks{Blocks: []dto.UserGroupBlock{
		{Group: "expired", ExpiresAt: now - 1},
		{Group: "temp", ExpiresAt: now + 3600},
		{Group: "forever", ExpiresAt: 0},
	}}

	_, ok := blocks.ActiveBlock("expired", now)
	require.False(t, ok, "expired block must not take effect")
	block, ok := blocks.ActiveBlock("temp", now)
	require.True(t, ok)
	require.Equal(t, now+3600, block.ExpiresAt)
	_, ok = blocks.ActiveBlock("forever", now)
	require.True(t, ok, "expires_at=0 means permanent")
	_, ok = blocks.ActiveBlock("other", now)
	require.False(t, ok)
	require.Len(t, blocks.ActiveBlocks(now), 2)
}

func TestCheckUserGroupBlockedSpecificGroup(t *testing.T) {
	setAutoGroupsForTest(t, `["default","vip"]`, `{"default":"","vip":""}`)
	expiresAt := time.Now().Unix() + 3600
	c := newGroupBlockContextForTest(dto.UserGroupBlock{Group: "vip", ExpiresAt: expiresAt})

	blocked, until := CheckUserGroupBlocked(c, "vip")
	require.True(t, blocked)
	require.Equal(t, expiresAt, until)

	blocked, _ = CheckUserGroupBlocked(c, "default")
	require.False(t, blocked, "other groups are unaffected")

	// auto 只封了部分分组：不整体拦截，选路时跳过被封分组
	blocked, _ = CheckUserGroupBlocked(c, "auto")
	require.False(t, blocked)
	require.Equal(t, []string{"default"}, GetRequestUserAutoGroup(c, "default"))
}

func TestCheckUserGroupBlockedAllAutoGroups(t *testing.T) {
	setAutoGroupsForTest(t, `["default","vip"]`, `{"default":"","vip":""}`)
	now := time.Now().Unix()

	// 一个永久、一个临时：最早恢复时间取临时的那个
	c := newGroupBlockContextForTest(
		dto.UserGroupBlock{Group: "default", ExpiresAt: 0},
		dto.UserGroupBlock{Group: "vip", ExpiresAt: now + 600},
	)
	blocked, until := CheckUserGroupBlocked(c, "auto")
	require.True(t, blocked)
	require.Equal(t, now+600, until)
	require.Empty(t, GetRequestUserAutoGroup(c, "default"))

	// 全部永久：恢复时间为 0
	c = newGroupBlockContextForTest(
		dto.UserGroupBlock{Group: "default", ExpiresAt: 0},
		dto.UserGroupBlock{Group: "vip", ExpiresAt: 0},
	)
	blocked, until = CheckUserGroupBlocked(c, "auto")
	require.True(t, blocked)
	require.Zero(t, until)
}

func TestCheckUserGroupBlockedExpired(t *testing.T) {
	setAutoGroupsForTest(t, `["default","vip"]`, `{"default":"","vip":""}`)
	c := newGroupBlockContextForTest(dto.UserGroupBlock{Group: "vip", ExpiresAt: time.Now().Unix() - 1})

	blocked, _ := CheckUserGroupBlocked(c, "vip")
	require.False(t, blocked, "block lifts automatically once expired")
	require.Equal(t, []string{"default", "vip"}, GetRequestUserAutoGroup(c, "default"))
}

func TestCheckUserGroupBlockedNoBlocks(t *testing.T) {
	setAutoGroupsForTest(t, `["default","vip"]`, `{"default":"","vip":""}`)
	c := newGroupBlockContextForTest()

	blocked, _ := CheckUserGroupBlocked(c, "vip")
	require.False(t, blocked)
	blocked, _ = CheckUserGroupBlocked(c, "auto")
	require.False(t, blocked)
	require.Equal(t, []string{"default", "vip"}, GetRequestUserAutoGroup(c, "default"))
}
