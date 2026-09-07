package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 渠道缓存口径归一：Claude 语义 prompt 不含缓存，OpenAI 语义 prompt 已含命中；
// 非聊天类入站格式不计入。用独占的渠道 id 读快照，避免与同包其他用例串扰。
func TestRecordChannelCacheUsageNormalizesBySemantic(t *testing.T) {
	tests := []struct {
		name        string
		channelId   int
		relayFormat types.RelayFormat
		summary     textQuotaSummary
		wantSamples int64
		wantInput   int64
		wantRead    int64
		wantWrite   int64
	}{
		{
			name:        "claude semantic adds read and write back into input",
			channelId:   910001,
			relayFormat: types.RelayFormatClaude,
			summary: textQuotaSummary{
				PromptTokens:          200,
				CacheTokens:           700,
				CacheCreationTokens:   100,
				IsClaudeUsageSemantic: true,
			},
			wantSamples: 1,
			wantInput:   1000,
			wantRead:    700,
			wantWrite:   100,
		},
		{
			name:        "claude semantic prefers split 5m/1h write when larger",
			channelId:   910002,
			relayFormat: types.RelayFormatOpenAI,
			summary: textQuotaSummary{
				PromptTokens:          100,
				CacheTokens:           0,
				CacheCreationTokens:   30,
				CacheCreationTokens5m: 30,
				CacheCreationTokens1h: 20,
				IsClaudeUsageSemantic: true,
			},
			wantSamples: 1,
			wantInput:   150,
			wantRead:    0,
			wantWrite:   50,
		},
		{
			name:        "openai semantic keeps prompt as total and clamps read",
			channelId:   910003,
			relayFormat: types.RelayFormatOpenAIResponses,
			summary: textQuotaSummary{
				PromptTokens:        1000,
				CacheTokens:         1200,
				CacheCreationTokens: 40, // OpenAI 没有写入概念，忽略
			},
			wantSamples: 1,
			wantInput:   1000,
			wantRead:    1000,
			wantWrite:   0,
		},
		{
			name:        "embedding format is not counted",
			channelId:   910004,
			relayFormat: types.RelayFormatEmbedding,
			summary:     textQuotaSummary{PromptTokens: 500},
			wantSamples: 0,
		},
	}

	// 结算路径上 ChannelMeta 可能为空（如本地估算），不能 panic
	recordChannelCacheUsage(&relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude}, textQuotaSummary{PromptTokens: 10})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recordChannelCacheUsage(&relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{ChannelId: tt.channelId},
				RelayFormat: tt.relayFormat,
			}, tt.summary)

			window, ok := model.GetChannelMetricsSnapshot()[tt.channelId]
			if tt.wantSamples == 0 {
				require.False(t, ok, "未计入的请求不应产生快照")
				return
			}
			require.True(t, ok)
			assert.Equal(t, tt.wantSamples, window.Hour.CacheSamples)
			assert.Equal(t, tt.wantInput, window.Hour.CacheInputTokens)
			assert.Equal(t, tt.wantRead, window.Hour.CacheReadTokens)
			assert.Equal(t, tt.wantWrite, window.Hour.CacheWriteTokens)
		})
	}
}
