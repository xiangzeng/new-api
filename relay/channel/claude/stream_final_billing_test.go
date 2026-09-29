package claude

import (
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newStreamFinalTestContext() (*gin.Context, *relaycommon.RelayInfo, *ClaudeResponseInfo) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-opus-5-5"},
	}
	claudeInfo := &ClaudeResponseInfo{Usage: &dto.Usage{}}

	FormatClaudeResponseInfo(&dto.ClaudeResponse{
		Type: "message_start",
		Message: &dto.ClaudeMediaMessage{
			Id:    "msg_1",
			Model: "claude-opus-5-5",
			Usage: &dto.ClaudeUsage{
				InputTokens:              100,
				OutputTokens:             1,
				CacheReadInputTokens:     30,
				CacheCreationInputTokens: 50,
			},
		},
	}, nil, claudeInfo)
	return c, info, claudeInfo
}

func feedDelta(claudeInfo *ClaudeResponseInfo, delta *dto.ClaudeMediaMessage) {
	FormatClaudeResponseInfo(&dto.ClaudeResponse{Type: "content_block_delta", Delta: delta}, nil, claudeInfo)
}

// 客户端中途断开（没有 message_delta）：结算用量必须带上已下发的推理/正文，而不是 message_start 的 1
func TestHandleStreamFinalResponse_AbnormalEndBillsStreamedOutput(t *testing.T) {
	c, info, claudeInfo := newStreamFinalTestContext()
	feedDelta(claudeInfo, &dto.ClaudeMediaMessage{Thinking: commonPointer(strings.Repeat("let me think about it. ", 200))})
	feedDelta(claudeInfo, &dto.ClaudeMediaMessage{Text: commonPointer("partial answer")})
	require.False(t, claudeInfo.Done)

	HandleStreamFinalResponse(c, info, claudeInfo)

	require.Greater(t, claudeInfo.Usage.CompletionTokens, 1)
	require.NotNil(t, claudeInfo.Usage.BillingUsage)
	billed := claudeInfo.Usage.BillingUsage.ClaudeUsage
	require.NotNil(t, billed)
	require.Equal(t, claudeInfo.Usage.CompletionTokens, billed.OutputTokens)
	// message_start 拿到的输入与缓存字段不能丢
	require.Equal(t, 100, billed.InputTokens)
	require.Equal(t, 30, billed.CacheReadInputTokens)
	require.Equal(t, 50, billed.CacheCreationInputTokens)
}

// 工具调用参数流到一半客户端断开：工具名与已下发的参数 JSON 同样计入输出
func TestHandleStreamFinalResponse_AbnormalEndBillsStreamedToolUse(t *testing.T) {
	c, info, claudeInfo := newStreamFinalTestContext()
	FormatClaudeResponseInfo(&dto.ClaudeResponse{
		Type:         "content_block_start",
		ContentBlock: &dto.ClaudeMediaMessage{Type: "tool_use", Id: "toolu_1", Name: "Write", Input: map[string]any{}},
	}, nil, claudeInfo)
	feedDelta(claudeInfo, &dto.ClaudeMediaMessage{Type: "input_json_delta", PartialJson: commonPointer(`{"file_path":"/tmp/a.go","content":"`)})
	feedDelta(claudeInfo, &dto.ClaudeMediaMessage{Type: "input_json_delta", PartialJson: commonPointer(strings.Repeat("package main\\nfunc main() {}\\n", 50))})
	require.False(t, claudeInfo.Done)

	HandleStreamFinalResponse(c, info, claudeInfo)

	require.Greater(t, claudeInfo.Usage.CompletionTokens, 1)
	billed := claudeInfo.Usage.BillingUsage.ClaudeUsage
	require.NotNil(t, billed)
	require.Equal(t, claudeInfo.Usage.CompletionTokens, billed.OutputTokens)
}

// 正常收尾：以上游 message_delta 的 output_tokens 为准，不被本地估算覆盖
func TestHandleStreamFinalResponse_NormalEndKeepsUpstreamOutput(t *testing.T) {
	c, info, claudeInfo := newStreamFinalTestContext()
	feedDelta(claudeInfo, &dto.ClaudeMediaMessage{Thinking: commonPointer(strings.Repeat("let me think about it. ", 200))})
	FormatClaudeResponseInfo(&dto.ClaudeResponse{
		Type:  "message_delta",
		Usage: &dto.ClaudeUsage{OutputTokens: 53},
	}, nil, claudeInfo)
	require.True(t, claudeInfo.Done)

	HandleStreamFinalResponse(c, info, claudeInfo)

	require.Equal(t, 53, claudeInfo.Usage.CompletionTokens)
	billed := claudeInfo.Usage.BillingUsage.ClaudeUsage
	require.Equal(t, 53, billed.OutputTokens)
	require.Equal(t, 100, billed.InputTokens)
	require.Equal(t, 30, billed.CacheReadInputTokens)
	require.Equal(t, 50, billed.CacheCreationInputTokens)
}
