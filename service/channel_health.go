package service

import (
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// IsChannelFaultError 判断错误是否属于「渠道故障」：级联模式下触发切换 + 熔断计数。
//
// 默认视为渠道故障：网络/连接类错误、429、401、403、5xx。
// 默认不处理（原样返回用户）：400 等其余状态码。
// 白名单可扩展（extra_fault_status_codes / extra_fault_keywords），
// ignore_fault_keywords 反向排除且优先级最高。
// 详见 docs/channel/cascade-failover.md
func IsChannelFaultError(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	setting := operation_setting.GetCascadeSetting()

	lowerMessage := strings.ToLower(err.Error())
	if len(setting.IgnoreFaultKeywords) > 0 {
		if hit, _ := AcSearch(lowerMessage, setting.IgnoreFaultKeywords, true); hit {
			return false
		}
	}

	if types.IsChannelError(err) {
		return true
	}

	code := err.StatusCode
	switch {
	case code == 429, code == 401, code == 403:
		return true
	case code >= 500 && code <= 599:
		return true
	}

	for _, extraCode := range setting.ExtraFaultStatusCodes {
		if code == extraCode {
			return true
		}
	}
	if len(setting.ExtraFaultKeywords) > 0 {
		if hit, _ := AcSearch(lowerMessage, setting.ExtraFaultKeywords, true); hit {
			return true
		}
	}
	return false
}

// StreamOutcome 流式请求「表面成功」（未返回错误对象）时的健康判定结果
type StreamOutcome int

const (
	// StreamOutcomeSuccess 正常结束，计入恢复
	StreamOutcomeSuccess StreamOutcome = iota
	// StreamOutcomeNeutral 无法归因于渠道（客户端断开/内部 panic 等），不计成功也不计故障
	StreamOutcomeNeutral
	// StreamOutcomeFault 渠道故障型断流，计入熔断
	StreamOutcomeFault
)

// ClassifyStreamEnd 对未返回错误对象的（流式）请求做健康归因。
// 流已向客户端吐字后失败无法透明切换，但仍要计入熔断标记，
// 让用户重发时绕开故障渠道。详见 docs/channel/cascade-failover.md
func ClassifyStreamEnd(status *relaycommon.StreamStatus) (StreamOutcome, string) {
	if status == nil {
		// 非流式请求或未走通用扫描器的路径，维持成功语义
		return StreamOutcomeSuccess, ""
	}
	switch status.EndReason {
	case relaycommon.StreamEndReasonTimeout:
		return StreamOutcomeFault, "流式响应超时（上游停止发送数据）"
	case relaycommon.StreamEndReasonScannerErr:
		reason := "流读取错误"
		if status.EndError != nil {
			reason += ": " + status.EndError.Error()
		}
		return StreamOutcomeFault, reason
	case relaycommon.StreamEndReasonEOF:
		if operation_setting.GetCascadeSetting().IncompleteStreamAsFault &&
			status.TracksCompletion() && !status.HasCompletion() {
			return StreamOutcomeFault, "上游中途断流（未收到协议完成标记）"
		}
		return StreamOutcomeSuccess, ""
	case relaycommon.StreamEndReasonClientGone,
		relaycommon.StreamEndReasonPanic,
		relaycommon.StreamEndReasonPingFail:
		// 客户端断开/内部异常，无法归因于渠道
		return StreamOutcomeNeutral, ""
	}
	return StreamOutcomeSuccess, ""
}
