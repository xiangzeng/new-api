package service

import (
	"errors"
	"net/http"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func withCascadeSetting(t *testing.T) *operation_setting.CascadeSetting {
	t.Helper()
	setting := operation_setting.GetCascadeSetting()
	orig := *setting
	t.Cleanup(func() { *setting = orig })
	setting.Enabled = true
	setting.ExtraFaultStatusCodes = nil
	setting.ExtraFaultKeywords = nil
	setting.IgnoreFaultKeywords = nil
	setting.CapacityDetourKeywords = nil
	return setting
}

func newStatusError(status int, message string) *types.NewAPIError {
	return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeBadResponse, status)
}

func TestIsChannelFaultErrorDefaults(t *testing.T) {
	withCascadeSetting(t)

	cases := []struct {
		status int
		want   bool
	}{
		{http.StatusTooManyRequests, true},
		{http.StatusUnauthorized, true},
		{http.StatusForbidden, true},
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
		{http.StatusBadRequest, false},
		{http.StatusNotFound, false},
		{http.StatusPaymentRequired, false},
	}
	for _, tc := range cases {
		if got := IsChannelFaultError(newStatusError(tc.status, "err")); got != tc.want {
			t.Errorf("status %d: want %v, got %v", tc.status, tc.want, got)
		}
	}
	if IsChannelFaultError(nil) {
		t.Error("nil error should not be a fault")
	}
}

func TestIsChannelFaultErrorExtraStatusCodes(t *testing.T) {
	setting := withCascadeSetting(t)
	setting.ExtraFaultStatusCodes = []int{http.StatusBadRequest}

	if !IsChannelFaultError(newStatusError(http.StatusBadRequest, "err")) {
		t.Error("whitelisted status code should be a fault")
	}
}

func TestIsChannelFaultErrorKeywords(t *testing.T) {
	setting := withCascadeSetting(t)
	setting.ExtraFaultKeywords = []string{"credential expired"}

	if !IsChannelFaultError(newStatusError(http.StatusBadRequest, "Credential Expired, please rebind")) {
		t.Error("keyword-matched 400 should be a fault")
	}
	if IsChannelFaultError(newStatusError(http.StatusBadRequest, "invalid request body")) {
		t.Error("plain 400 should not be a fault")
	}
}

func TestIsChannelFaultErrorIgnoreKeywordsPrecedence(t *testing.T) {
	setting := withCascadeSetting(t)
	setting.IgnoreFaultKeywords = []string{"quota exceeded for user"}

	if IsChannelFaultError(newStatusError(http.StatusTooManyRequests, "Quota Exceeded For User abc")) {
		t.Error("ignore keyword should take precedence over 429")
	}
}

// 生产实例：Kiro 号商的输入上限报错（2026-10-17 渠道 #58）
const nativeInputBudgetMessage = "Latest complete request and protected instructions require approximately 136875 input tokens, exceeding the native input budget 124000. No messages were truncated."

func TestIsChannelCapacityError(t *testing.T) {
	setting := withCascadeSetting(t)

	if IsChannelCapacityError(newStatusError(http.StatusBadRequest, nativeInputBudgetMessage)) {
		t.Error("no keywords configured: should not be a capacity error")
	}

	setting.CapacityDetourKeywords = []string{"Native Input Budget"}
	if !IsChannelCapacityError(newStatusError(http.StatusBadRequest, nativeInputBudgetMessage)) {
		t.Error("keyword-matched error should be a capacity error (case-insensitive)")
	}
	if IsChannelCapacityError(newStatusError(http.StatusBadRequest, "invalid request body")) {
		t.Error("plain 400 should not be a capacity error")
	}
	if IsChannelCapacityError(nil) {
		t.Error("nil error should not be a capacity error")
	}
}

func TestIsChannelFaultErrorCapacityPrecedence(t *testing.T) {
	setting := withCascadeSetting(t)
	setting.CapacityDetourKeywords = []string{"native input budget"}
	setting.ExtraFaultKeywords = []string{"native input budget"}

	if IsChannelFaultError(newStatusError(http.StatusBadRequest, nativeInputBudgetMessage)) {
		t.Error("capacity keyword should take precedence over extra fault keywords")
	}
	if IsChannelFaultError(newStatusError(http.StatusInternalServerError, nativeInputBudgetMessage)) {
		t.Error("capacity keyword should take precedence over 5xx")
	}
	if !IsChannelFaultError(newStatusError(http.StatusInternalServerError, "upstream exploded")) {
		t.Error("unrelated 5xx should still be a fault")
	}
}

func TestCascadeSettingDefaultCapacityKeywords(t *testing.T) {
	// 默认值预填已知的号商输入上限报错，部署后无需手动配置即生效
	defaults := operation_setting.GetCascadeSetting().CapacityDetourKeywords
	found := false
	for _, keyword := range defaults {
		if keyword == "native input budget" {
			found = true
		}
	}
	if !found {
		t.Errorf("default capacity keywords = %v, want to contain %q", defaults, "native input budget")
	}
}

func TestClassifyStreamEnd(t *testing.T) {
	setting := withCascadeSetting(t)
	setting.IncompleteStreamAsFault = true

	// 非流式请求（无 StreamStatus）→ 成功
	if outcome, _ := ClassifyStreamEnd(nil); outcome != StreamOutcomeSuccess {
		t.Error("nil status should be success")
	}

	makeStatus := func(reason relaycommon.StreamEndReason) *relaycommon.StreamStatus {
		s := relaycommon.NewStreamStatus()
		s.SetEndReason(reason, errors.New("x"))
		return s
	}

	// 无歧义故障：超时 / 读错误
	if outcome, _ := ClassifyStreamEnd(makeStatus(relaycommon.StreamEndReasonTimeout)); outcome != StreamOutcomeFault {
		t.Error("timeout should be fault")
	}
	if outcome, _ := ClassifyStreamEnd(makeStatus(relaycommon.StreamEndReasonScannerErr)); outcome != StreamOutcomeFault {
		t.Error("scanner error should be fault")
	}

	// 中性：客户端断开 / panic
	if outcome, _ := ClassifyStreamEnd(makeStatus(relaycommon.StreamEndReasonClientGone)); outcome != StreamOutcomeNeutral {
		t.Error("client gone should be neutral")
	}
	if outcome, _ := ClassifyStreamEnd(makeStatus(relaycommon.StreamEndReasonPanic)); outcome != StreamOutcomeNeutral {
		t.Error("panic should be neutral")
	}

	// 正常结束
	if outcome, _ := ClassifyStreamEnd(makeStatus(relaycommon.StreamEndReasonDone)); outcome != StreamOutcomeSuccess {
		t.Error("done should be success")
	}
}

func TestClassifyStreamEndIncompleteEOF(t *testing.T) {
	setting := withCascadeSetting(t)
	setting.IncompleteStreamAsFault = true

	// EOF + 支持完成跟踪 + 未收到完成标记 → 安静断流，故障
	s := relaycommon.NewStreamStatus()
	s.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
	s.EnableCompletionTracking()
	if outcome, _ := ClassifyStreamEnd(s); outcome != StreamOutcomeFault {
		t.Error("EOF without completion marker should be fault")
	}

	// EOF + 收到完成标记 → 正常（Claude 正常流就是 EOF 结束）
	s2 := relaycommon.NewStreamStatus()
	s2.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
	s2.EnableCompletionTracking()
	s2.MarkCompletion()
	if outcome, _ := ClassifyStreamEnd(s2); outcome != StreamOutcomeSuccess {
		t.Error("EOF with completion marker should be success")
	}

	// EOF + 适配器不支持完成跟踪（如 Gemini）→ 维持成功，不误伤
	s3 := relaycommon.NewStreamStatus()
	s3.SetEndReason(relaycommon.StreamEndReasonEOF, nil)
	if outcome, _ := ClassifyStreamEnd(s3); outcome != StreamOutcomeSuccess {
		t.Error("EOF without tracking support should be success")
	}

	// 开关关闭 → EOF 不计故障
	setting.IncompleteStreamAsFault = false
	if outcome, _ := ClassifyStreamEnd(s); outcome != StreamOutcomeSuccess {
		t.Error("EOF should be success when the switch is off")
	}
}
