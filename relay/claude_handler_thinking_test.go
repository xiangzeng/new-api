package relay

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeModelRejectsBudgetThinking(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{model: "claude-opus-5-5", want: true},
		{model: "claude-opus-5", want: true},
		{model: "claude-sonnet-5-5", want: true},
		{model: "claude-sonnet-5", want: true},
		{model: "claude-opus-4-8", want: true},
		{model: "claude-opus-4-7", want: true},
		{model: "claude-haiku-5-5", want: true},
		{model: "claude-fable-5-1", want: true},
		{model: "claude-opus-4-6", want: false},
		{model: "claude-opus-4-5-20251101", want: false},
		{model: "claude-sonnet-4-5", want: false},
		{model: "claude-sonnet-4-6", want: false},
		{model: "claude-haiku-4-5", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.want, claudeModelRejectsBudgetThinking(tt.model))
		})
	}
}

func TestNormalizeClaudeLegacyThinkingRewritesEnabled(t *testing.T) {
	request := &dto.ClaudeRequest{
		Model:        "claude-opus-5-5",
		Thinking:     &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer[int](4096)},
		Temperature:  common.GetPointer[float64](1.0),
		TopP:         common.GetPointer[float64](0.95),
		TopK:         common.GetPointer[int](40),
		OutputConfig: json.RawMessage(`{"effort":"low"}`),
	}

	normalizeClaudeLegacyThinking(request)

	require.NotNil(t, request.Thinking)
	assert.Equal(t, "adaptive", request.Thinking.Type)
	assert.Nil(t, request.Thinking.BudgetTokens)
	assert.Empty(t, request.Thinking.Display)
	assert.Nil(t, request.Temperature)
	assert.Nil(t, request.TopP)
	assert.Nil(t, request.TopK)
	assert.Equal(t, "low", request.GetEfforts())

	body, err := common.Marshal(request)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "budget_tokens")
	assert.Contains(t, string(body), `"thinking":{"type":"adaptive"}`)
}

func TestNormalizeClaudeLegacyThinkingKeepsDisplay(t *testing.T) {
	request := &dto.ClaudeRequest{
		Model:    "claude-sonnet-5-5",
		Thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer[int](2048), Display: "summarized"},
	}

	normalizeClaudeLegacyThinking(request)

	assert.Equal(t, &dto.Thinking{Type: "adaptive", Display: "summarized"}, request.Thinking)
	assert.Nil(t, request.OutputConfig)
}

func TestNormalizeClaudeLegacyThinkingLeavesOthersUntouched(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		thinking *dto.Thinking
	}{
		{name: "budget model keeps enabled", model: "claude-opus-4-6", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer[int](2048)}},
		{name: "older sonnet keeps enabled", model: "claude-sonnet-4-5", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer[int](2048)}},
		{name: "adaptive unchanged", model: "claude-opus-5-5", thinking: &dto.Thinking{Type: "adaptive", Display: "summarized"}},
		{name: "disabled unchanged", model: "claude-opus-4-8", thinking: &dto.Thinking{Type: "disabled"}},
		{name: "no thinking", model: "claude-opus-5-5", thinking: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			temperature := common.GetPointer[float64](0.7)
			request := &dto.ClaudeRequest{Model: tt.model, Thinking: tt.thinking, Temperature: temperature}

			normalizeClaudeLegacyThinking(request)

			assert.Same(t, tt.thinking, request.Thinking)
			assert.Same(t, temperature, request.Temperature)
		})
	}
}
