package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// 级联熔断渠道的探活恢复循环。
// 只探「熔断中」且未被手动禁用的渠道（健康渠道由真实流量验证，零探活成本），
// 连续 recovery_success_count 次成功后恢复，期间任一失败清零重数。
// 详见 docs/channel/cascade-failover.md

// CascadeProbeTokenName 探活请求在消耗日志中的令牌名，用于把探活流量从模型测试/真实流量里筛出来
const CascadeProbeTokenName = "熔断探活"

type cascadeProbeCtxKey struct{}

// withCascadeProbe 给探活发起的渠道测试打标记，日志侧据此区分探活与手动模型测试
func withCascadeProbe(ctx context.Context) context.Context {
	return context.WithValue(ctx, cascadeProbeCtxKey{}, true)
}

// isCascadeProbe 判断当前渠道测试是否由级联探活发起
func isCascadeProbe(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	probe, ok := ctx.Value(cascadeProbeCtxKey{}).(bool)
	return ok && probe
}

// StartCascadeHealthProbeLoop 启动探活循环（main.go 调用，常驻 goroutine）
func StartCascadeHealthProbeLoop() {
	common.SysLog("cascade health probe loop started")
	for {
		setting := operation_setting.GetCascadeSetting()
		time.Sleep(time.Duration(setting.GetProbeIntervalSeconds()) * time.Second)
		if !setting.Enabled || !setting.ProbeEnabled {
			continue
		}
		probeTrippedChannels()
	}
}

func probeTrippedChannels() {
	defer func() {
		if r := recover(); r != nil {
			common.SysError(fmt.Sprintf("cascade health probe panic: %v", r))
		}
	}()

	trippedIds := model.ListTrippedChannelIds()
	if len(trippedIds) == 0 {
		return
	}
	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		common.SysError("cascade health probe: " + err.Error())
		return
	}

	ctx := withCascadeProbe(context.Background())
	for _, channelId := range trippedIds {
		channel, err := model.GetChannelById(channelId, true)
		if err != nil || channel == nil {
			// 渠道已被删除，清除残留标记
			model.ResetChannelHealth(channelId)
			continue
		}
		if channel.Status != common.ChannelStatusEnabled {
			// 手动禁用的渠道不参与探活
			continue
		}

		// 探活流量不计入渠道指标（attempts/faults），避免稀释真实流量口径；
		// 但探活促成的恢复是状态变迁，照记 restore
		result := testChannel(ctx, channel, testUserID, "", "", shouldUseStreamForAutomaticChannelTest(channel))
		if result.localErr == nil && result.newAPIError == nil {
			if model.MarkChannelHealthSuccess(channelId) {
				model.RecordChannelRestore(channelId)
				model.AppendChannelHealthEvent(channelId, model.ChannelHealthEventRestore, "探活连续成功恢复")
				common.SysLog(fmt.Sprintf("级联：渠道 #%d（%s）探活连续成功达标，恢复健康", channelId, channel.Name))
			}
		} else {
			errMsg := "probe failed"
			if result.newAPIError != nil {
				errMsg = result.newAPIError.ErrorWithStatusCode()
			} else if result.localErr != nil {
				errMsg = result.localErr.Error()
			}
			model.MarkChannelHealthFailure(channelId, "探活失败: "+errMsg)
		}

		if common.RequestInterval > 0 {
			time.Sleep(common.RequestInterval)
		}
	}
}
