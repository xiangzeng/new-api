package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

const resellerCommissionReleaseBatchSize = 200

type resellerCommissionReleaseHandler struct{}

type ResellerCommissionReleaseState struct {
	Processed int   `json:"processed"`
	Remaining int64 `json:"remaining"`
	LastId    int64 `json:"last_id"`
	Progress  int   `json:"progress"`
}

type ResellerCommissionReleaseResult struct {
	Released int `json:"released"`
}

func (resellerCommissionReleaseHandler) Type() string {
	return model.SystemTaskTypeResellerCommissionRelease
}

func (resellerCommissionReleaseHandler) Enabled() bool {
	return model.HasDueResellerCommissions(time.Now().Unix())
}

func (resellerCommissionReleaseHandler) Interval() time.Duration { return time.Minute }

func (resellerCommissionReleaseHandler) NewPayload() any { return nil }

func (resellerCommissionReleaseHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	state := ResellerCommissionReleaseState{}
	if err := task.DecodeState(&state); err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	cutoff := time.Now().Unix()
	for {
		batch, err := model.ReleaseDueResellerCommissionsBatch(ctx, cutoff, resellerCommissionReleaseBatchSize)
		if err != nil {
			if ctx.Err() != nil {
				logger.LogWarn(ctx, fmt.Sprintf("reseller commission release task %s canceled", task.TaskID))
				return
			}
			failSystemTask(task, runnerID, err)
			return
		}
		state.Processed += batch.Processed
		state.Remaining = batch.Remaining
		if batch.LastId > 0 {
			state.LastId = batch.LastId
		}
		if state.Remaining == 0 {
			state.Progress = 100
		} else {
			state.Progress = 50
		}
		if err := model.UpdateSystemTaskState(task.TaskID, runnerID, state); err != nil {
			logSystemTaskLockError(ctx, task, err)
			return
		}
		if batch.Remaining == 0 {
			break
		}
		if batch.Processed == 0 {
			failSystemTask(task, runnerID, fmt.Errorf("reseller commission release made no progress with %d rows remaining", batch.Remaining))
			return
		}
	}

	result := ResellerCommissionReleaseResult{Released: state.Processed}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, result, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func init() {
	RegisterSystemTaskHandler(resellerCommissionReleaseHandler{})
}
