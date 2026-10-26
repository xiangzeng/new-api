package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFormatUserLogsStripsQuotaSaturation verifies the admin-only quota
// saturation marker (nested under other.admin_info) is removed for non-admin
// log views, since formatUserLogs strips the whole admin_info object.
func TestFormatUserLogsStripsQuotaSaturation(t *testing.T) {
	other := common.MapToJsonStr(map[string]interface{}{
		"model_price": 0.004,
		"admin_info": map[string]interface{}{
			"quota_saturation": map[string]interface{}{
				"op":      "QuotaFromDecimal",
				"kind":    "overflow",
				"clamped": common.MaxQuota,
			},
		},
	})
	logs := []*Log{{Other: other}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	_, hasAdminInfo := parsed["admin_info"]
	require.False(t, hasAdminInfo, "admin_info (and nested quota_saturation) must be stripped for non-admin views")
	// Non-admin billing fields remain visible.
	require.Contains(t, parsed, "model_price")
}

// TestFormatUserLogsStripsChannelIdentity verifies that error logs, which record
// the serving channel at the top level of other, do not expose that channel or
// the admin reject reason to the log owner, while the error details stay visible.
func TestFormatUserLogsStripsChannelIdentity(t *testing.T) {
	other := common.MapToJsonStr(map[string]interface{}{
		"request_path":  "/v1/messages",
		"error_type":    "upstream_error",
		"status_code":   502,
		"channel_id":    21,
		"channel_name":  "kiro-primary",
		"channel_type":  14,
		"reject_reason": "blocked by admin rule",
	})
	logs := []*Log{{Other: other, ChannelId: 21, ChannelName: "kiro-primary"}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	for _, key := range []string{"channel_id", "channel_name", "channel_type", "reject_reason"} {
		assert.NotContains(t, parsed, key)
	}
	assert.Equal(t, "/v1/messages", parsed["request_path"])
	assert.Equal(t, "upstream_error", parsed["error_type"])
	assert.EqualValues(t, 502, parsed["status_code"])
	assert.Empty(t, logs[0].ChannelName)
}
