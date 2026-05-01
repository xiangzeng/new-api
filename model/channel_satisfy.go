package model

import (
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

type ChannelCandidate struct {
	Id     int
	Status int
}

type ChannelSelectionDiagnostic struct {
	Group                 string
	Model                 string
	AvailableChannelIds   []int
	UnavailableCandidates []ChannelCandidate
}

func IsChannelEnabledForGroupModel(group string, modelName string, channelID int) bool {
	if group == "" || modelName == "" || channelID <= 0 {
		return false
	}
	if !common.MemoryCacheEnabled {
		return isChannelEnabledForGroupModelDB(group, modelName, channelID)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	if group2model2channels == nil {
		return false
	}

	if isChannelIDInList(group2model2channels[group][modelName], channelID) {
		return true
	}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized != "" && normalized != modelName {
		return isChannelIDInList(group2model2channels[group][normalized], channelID)
	}
	return false
}

func IsChannelEnabledForAnyGroupModel(groups []string, modelName string, channelID int) bool {
	if len(groups) == 0 {
		return false
	}
	for _, g := range groups {
		if IsChannelEnabledForGroupModel(g, modelName, channelID) {
			return true
		}
	}
	return false
}

func isChannelEnabledForGroupModelDB(group string, modelName string, channelID int) bool {
	var count int64
	err := DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? and model = ? and channel_id = ? and enabled = ?", group, modelName, channelID, true).
		Count(&count).Error
	if err == nil && count > 0 {
		return true
	}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized == "" || normalized == modelName {
		return false
	}
	count = 0
	err = DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? and model = ? and channel_id = ? and enabled = ?", group, normalized, channelID, true).
		Count(&count).Error
	return err == nil && count > 0
}

func isChannelIDInList(list []int, channelID int) bool {
	for _, id := range list {
		if id == channelID {
			return true
		}
	}
	return false
}

func GetChannelSelectionDiagnostic(group string, modelName string) (ChannelSelectionDiagnostic, error) {
	diagnostic := ChannelSelectionDiagnostic{
		Group: group,
		Model: modelName,
	}
	if group == "" || modelName == "" {
		return diagnostic, nil
	}

	models := []string{modelName}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized != "" && normalized != modelName {
		models = append(models, normalized)
	}

	type channelCandidateRow struct {
		ChannelId     int
		AbilityEnable bool
		ChannelStatus int
	}
	var rows []channelCandidateRow
	err := DB.Table("abilities").
		Select("abilities.channel_id, abilities.enabled as ability_enable, channels.status as channel_status").
		Joins("left join channels on channels.id = abilities.channel_id").
		Where("abilities."+commonGroupCol+" = ? and abilities.model in ?", group, models).
		Scan(&rows).Error
	if err != nil {
		return diagnostic, err
	}

	available := make(map[int]struct{})
	unavailable := make(map[int]int)
	for _, row := range rows {
		if row.ChannelId <= 0 {
			continue
		}
		if row.AbilityEnable && row.ChannelStatus == common.ChannelStatusEnabled {
			available[row.ChannelId] = struct{}{}
			delete(unavailable, row.ChannelId)
			continue
		}
		if _, ok := available[row.ChannelId]; !ok {
			unavailable[row.ChannelId] = row.ChannelStatus
		}
	}

	for id := range available {
		diagnostic.AvailableChannelIds = append(diagnostic.AvailableChannelIds, id)
	}
	sort.Ints(diagnostic.AvailableChannelIds)

	unavailableIds := make([]int, 0, len(unavailable))
	for id := range unavailable {
		unavailableIds = append(unavailableIds, id)
	}
	sort.Ints(unavailableIds)
	for _, id := range unavailableIds {
		diagnostic.UnavailableCandidates = append(diagnostic.UnavailableCandidates, ChannelCandidate{
			Id:     id,
			Status: unavailable[id],
		})
	}
	return diagnostic, nil
}

func FormatChannelCandidates(candidates []ChannelCandidate) string {
	if len(candidates) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		parts = append(parts, fmt.Sprintf("%d(status=%d)", candidate.Id, candidate.Status))
	}
	return "[" + strings.Join(parts, ",") + "]"
}
