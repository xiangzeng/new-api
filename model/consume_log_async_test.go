package model

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newConsumeLogTestContext(t *testing.T, username string, requestId string, remoteAddr string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req.RemoteAddr = remoteAddr
	c.Request = req
	c.Set("username", username)
	c.Set(common.RequestIdKey, requestId)
	c.Set(common.UpstreamRequestIdKey, "upstream-"+requestId)
	return c
}

func useAsyncConsumeLog(t *testing.T) {
	t.Helper()
	previous := common.ConsumeLogAsync
	common.ConsumeLogAsync = true
	t.Cleanup(func() {
		require.True(t, FlushConsumeLogs(5*time.Second))
		common.ConsumeLogAsync = previous
	})
}

func TestAsyncConsumeLogKeepsRequestValuesAfterContextReuse(t *testing.T) {
	truncateTables(t)
	useAsyncConsumeLog(t)
	user := User{Username: "async-log-user", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Group: "default", Setting: `{"record_ip_log":true}`}
	require.NoError(t, DB.Create(&user).Error)

	c := newConsumeLogTestContext(t, user.Username, "async-req-1", "203.0.113.7:4321")
	before := common.GetTimestamp()
	RecordConsumeLog(c, user.Id, RecordConsumeLogParams{
		ChannelId: 22, PromptTokens: 45, CompletionTokens: 14, ModelName: "claude-sonnet-4-6",
		TokenName: "t", Quota: 174, TokenId: 9, IsStream: true, Group: "claude-kiro",
		Other: map[string]interface{}{"group_ratio": 1},
	})
	// 模拟 handler 返回后 gin 回收并复用这个 Context
	c.Set("username", "someone-else")
	c.Set(common.RequestIdKey, "reused-request")
	c.Request.RemoteAddr = "198.51.100.1:1"

	require.True(t, FlushConsumeLogs(5*time.Second))
	var logs []Log
	require.NoError(t, LOG_DB.Where("request_id = ?", "async-req-1").Find(&logs).Error)
	require.Len(t, logs, 1)
	got := logs[0]
	assert.Equal(t, LogTypeConsume, got.Type)
	assert.Equal(t, user.Username, got.Username)
	assert.Equal(t, "upstream-async-req-1", got.UpstreamRequestId)
	assert.Equal(t, "203.0.113.7", got.Ip, "record_ip_log user must get the original client IP")
	assert.Equal(t, 174, got.Quota)
	assert.Equal(t, 22, got.ChannelId)
	assert.True(t, got.IsStream)
	assert.GreaterOrEqual(t, got.CreatedAt, before)
	assert.LessOrEqual(t, got.CreatedAt, before+1, "created_at is the call time, not the write time")
	var reused int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("request_id = ?", "reused-request").Count(&reused).Error)
	assert.Zero(t, reused)
}

func TestAsyncConsumeLogFlushWritesEveryQueuedEntry(t *testing.T) {
	truncateTables(t)
	useAsyncConsumeLog(t)
	user := User{Username: "async-log-burst", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, DB.Create(&user).Error)

	// 超过队列容量，覆盖「队列满时当场写」的分支
	const total = consumeLogQueueSize + 200
	c := newConsumeLogTestContext(t, user.Username, "burst", "203.0.113.8:1")
	for i := 0; i < total; i++ {
		RecordConsumeLog(c, user.Id, RecordConsumeLogParams{ModelName: "m", Quota: 1, ChannelId: 1})
	}
	require.True(t, FlushConsumeLogs(30*time.Second))
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", user.Id, LogTypeConsume).Count(&count).Error)
	assert.EqualValues(t, total, count)
	var withIp int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND ip <> ''", user.Id).Count(&withIp).Error)
	assert.Zero(t, withIp, "users without record_ip_log must not get an IP")
}

func TestSyncConsumeLogIsWrittenBeforeReturn(t *testing.T) {
	truncateTables(t)
	require.False(t, common.ConsumeLogAsync, "async consume log must be off by default")
	user := User{Username: "sync-log-user", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, DB.Create(&user).Error)

	c := newConsumeLogTestContext(t, user.Username, "sync-req-1", "203.0.113.9:1")
	RecordConsumeLog(c, user.Id, RecordConsumeLogParams{ModelName: "m", Quota: 7})
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("request_id = ?", "sync-req-1").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}
