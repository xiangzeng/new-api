package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPreRelayErrorLogTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis, previousErrorLog := common.RedisEnabled, constant.ErrorLogEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled = false
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled, constant.ErrorLogEnabled = previousRedis, previousErrorLog
	})
}

// rejectAfterAuth 模拟鉴权已认出用户（或没认出）之后被网关拒绝的请求。
func rejectAfterAuth(t *testing.T, routeTag string, userId int) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.POST("/v1/messages", RouteTag(routeTag), func(c *gin.Context) {
		if userId != 0 {
			c.Set("id", userId)
			c.Set("token_id", 7)
			c.Set("token_name", "kiro")
			c.Set("group", "claude-kiro")
		}
		abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "当前分组 claude-kiro 下对于模型 claude-haiku-4-5 无可用渠道", types.ErrorCodeModelNotFound)
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-haiku-4-5"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func errorLogs(t *testing.T) []model.Log {
	t.Helper()
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeError).Find(&logs).Error)
	return logs
}

// 认得出用户的转发请求在转发前被拒绝时，用户要能在使用日志的错误记录里看到原因。
func TestRejectedBeforeRelayIsRecordedInErrorLog(t *testing.T) {
	setupPreRelayErrorLogTest(t)

	recorder := rejectAfterAuth(t, "relay", 42)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)

	logs := errorLogs(t)
	require.Len(t, logs, 1)
	assert.Equal(t, 42, logs[0].UserId)
	assert.Equal(t, 7, logs[0].TokenId)
	assert.Equal(t, "kiro", logs[0].TokenName)
	assert.Equal(t, "claude-kiro", logs[0].Group)
	assert.Equal(t, "claude-haiku-4-5", logs[0].ModelName)
	assert.Equal(t, 0, logs[0].ChannelId)
	assert.True(t, strings.HasPrefix(logs[0].Content, "status_code=503, "))
	assert.Contains(t, logs[0].Other, `"rejected_before_relay":true`)
	assert.Contains(t, logs[0].Other, `"error_code":"model_not_found"`)
}

// 没认出用户（令牌不存在）或不是转发接口时不写日志。
func TestPreRelayRejectionWithoutUserOrOutsideRelayIsNotRecorded(t *testing.T) {
	setupPreRelayErrorLogTest(t)

	require.Equal(t, http.StatusServiceUnavailable, rejectAfterAuth(t, "relay", 0).Code)
	require.Equal(t, http.StatusServiceUnavailable, rejectAfterAuth(t, "api", 42).Code)

	assert.Empty(t, errorLogs(t))
}
