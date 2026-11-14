package middleware

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func abortWithOpenAiMessage(c *gin.Context, statusCode int, message string, code ...types.ErrorCode) {
	codeStr := ""
	if len(code) > 0 {
		codeStr = string(code[0])
	}
	userId := c.GetInt("id")
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
			"type":    "new_api_error",
			"code":    codeStr,
		},
	})
	c.Abort()
	logger.LogError(c.Request.Context(), fmt.Sprintf("user %d | %s", userId, message))
	recordPreRelayErrorLog(c, statusCode, message, codeStr)
}

func abortWithMidjourneyMessage(c *gin.Context, statusCode int, code int, description string) {
	c.JSON(statusCode, gin.H{
		"description": description,
		"type":        "new_api_error",
		"code":        code,
	})
	c.Abort()
	logger.LogError(c.Request.Context(), description)
}

// 转发前拒绝的错误日志限速：同一用户同一错误码每分钟最多记这么多条，
// 防止客户端拿失效令牌死循环重试、或系统过载时把日志表刷爆。
const preRelayErrorLogPerMinute = 60

var (
	preRelayErrorLogLimiter     common.InMemoryRateLimiter
	preRelayErrorLogLimiterOnce sync.Once
)

// recordPreRelayErrorLog 把在转发上游之前就被网关拒绝的请求（系统过载、令牌失效/额度用尽、
// IP 限制、分组无权、限流、无可用渠道等）写进使用日志的错误记录，用户在「仅错误」里能看到原因。
// 只处理已经认出用户的 relay 请求：令牌本身不存在时无从归属，不记。
func recordPreRelayErrorLog(c *gin.Context, statusCode int, message string, errorCode string) {
	if !constant.ErrorLogEnabled || c.GetString(RouteTagKey) != "relay" {
		return
	}
	userId := c.GetInt("id")
	if userId == 0 {
		return
	}
	preRelayErrorLogLimiterOnce.Do(func() {
		preRelayErrorLogLimiter.Init(5 * time.Minute)
	})
	if !preRelayErrorLogLimiter.Request(fmt.Sprintf("%d:%d:%s", userId, statusCode, errorCode), preRelayErrorLogPerMinute, 60) {
		return
	}

	modelName := c.GetString("original_model")
	if modelName == "" {
		if modelRequest, err := getModelFromRequest(c); err == nil && modelRequest != nil {
			modelName = modelRequest.Model
		}
	}
	group := c.GetString("group")
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}
	other := map[string]interface{}{
		"request_path":          c.Request.URL.Path,
		"error_type":            "new_api_error",
		"error_code":            errorCode,
		"status_code":           statusCode,
		"rejected_before_relay": true,
	}
	content := fmt.Sprintf("status_code=%d, %s", statusCode, message)
	if err := model.RecordErrorLog(c, userId, 0, modelName, c.GetString("token_name"), content, c.GetInt("token_id"), 0, false, group, other); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("record pre-relay error log failed: %v", err))
	}
}
