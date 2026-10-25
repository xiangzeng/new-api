package middleware

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

// channelInAnyGroup 决定归属渠道「不支持本次模型」时是绕行（仍在分组）还是失效清除（已移出分组）
func TestChannelInAnyGroup(t *testing.T) {
	ch := &model.Channel{Group: "kiro-高缓, kiro-特价"}

	require.True(t, channelInAnyGroup(ch, []string{"kiro-高缓"}), "指定分组命中")
	require.True(t, channelInAnyGroup(ch, []string{"default", "kiro-特价"}), "auto 多分组任一命中")
	require.False(t, channelInAnyGroup(ch, []string{"default"}), "已移出分组")
	require.False(t, channelInAnyGroup(ch, nil), "无候选分组")
	require.False(t, channelInAnyGroup(&model.Channel{Group: ""}, []string{""}), "渠道无分组")
	require.False(t, channelInAnyGroup(nil, []string{"kiro-高缓"}), "渠道为空")
}
