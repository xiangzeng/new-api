package model

import (
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// 级联选择器：按分组「编排顺序」（cascade_order.group_orders，与渠道优先级解耦）
// 严格依次遍历候选渠道，跳过已试过与熔断中的渠道；未配置顺序的分组、未入列的
// 渠道按优先级降序、id 升序兜底（与旧版行为一致）。
// 全部熔断时兜底：忽略健康标记仍按顺序选择，保证服务可用性优先于标记。
// 详见 docs/channel/cascade-failover.md

// GetCascadeSatisfiedChannel 级联模式下的渠道选择。
// excludeIds 为本次请求已经尝试过的渠道 ID 集合。
// 返回 nil 表示该分组下已无可尝试的渠道。
func GetCascadeSatisfiedChannel(group string, model string, requestPath string, excludeIds map[int]bool) (*Channel, error) {
	candidates, err := cascadeCandidateChannels(group, model, requestPath)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// 第一轮：只考虑健康且未试过的渠道
	if channel := pickCascadeChannel(candidates, excludeIds, true); channel != nil {
		return channel, nil
	}
	// 兜底轮：全部熔断时忽略健康标记，仍按顺序选一个未试过的渠道
	if channel := pickCascadeChannel(candidates, excludeIds, false); channel != nil {
		return channel, nil
	}
	return nil, nil
}

// CountCascadeCandidates 返回分组下候选渠道数量（级联模式的重试上限依据）
func CountCascadeCandidates(group string, model string, requestPath string) int {
	candidates, err := cascadeCandidateChannels(group, model, requestPath)
	if err != nil {
		return 0
	}
	return len(candidates)
}

// pickCascadeChannel 按候选列表顺序（已按编排顺序排好）返回第一个可用渠道。
// checkHealth 为 false 时忽略健康标记（兜底轮）。
func pickCascadeChannel(candidates []*Channel, excludeIds map[int]bool, checkHealth bool) *Channel {
	for _, channel := range candidates {
		if excludeIds[channel.Id] {
			continue
		}
		if checkHealth && !IsChannelHealthAvailable(channel.Id) {
			continue
		}
		return channel
	}
	return nil
}

// CascadeOrderPositions 返回分组编排顺序的 id→位置表（位置从 1 起，
// 0/缺失 = 未入列）。重复 id 取首次出现的位置。编排页 overview 复用。
func CascadeOrderPositions(group string) map[int]int {
	order := operation_setting.GetCascadeGroupOrder(group)
	pos := make(map[int]int, len(order))
	for i, id := range order {
		if _, ok := pos[id]; !ok {
			pos[id] = i + 1
		}
	}
	return pos
}

// sortCascadeCandidates 按分组编排顺序排序候选渠道：入列的按列表位置，
// 未入列的（新渠道/未配置分组）按优先级降序、id 升序垫底。
func sortCascadeCandidates(group string, candidates []*Channel) {
	pos := CascadeOrderPositions(group)
	sort.SliceStable(candidates, func(i, j int) bool {
		pi, pj := pos[candidates[i].Id], pos[candidates[j].Id]
		if pi != 0 || pj != 0 {
			if pi == 0 || pj == 0 {
				return pi != 0 // 入列的排未入列前面
			}
			return pi < pj
		}
		if candidates[i].GetPriority() != candidates[j].GetPriority() {
			return candidates[i].GetPriority() > candidates[j].GetPriority()
		}
		return candidates[i].Id < candidates[j].Id
	})
}

// cascadeCandidateChannels 返回分组+模型下的全部候选渠道（去重，优先级降序）。
// 内存缓存开启时走缓存，否则走数据库。
func cascadeCandidateChannels(group string, model string, requestPath string) ([]*Channel, error) {
	if common.MemoryCacheEnabled {
		return cascadeCandidatesFromCache(group, model, requestPath)
	}
	return cascadeCandidatesFromDB(group, model, requestPath)
}

func cascadeCandidatesFromCache(group string, model string, requestPath string) ([]*Channel, error) {
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	channelIds := filterChannelsByRequestPathAndModel(group2model2channels[group][model], requestPath, model)
	if len(channelIds) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		channelIds = filterChannelsByRequestPathAndModel(group2model2channels[group][normalizedModel], requestPath, model)
	}
	if len(channelIds) == 0 {
		return nil, nil
	}

	seen := make(map[int]bool, len(channelIds))
	candidates := make([]*Channel, 0, len(channelIds))
	for _, channelId := range channelIds {
		if seen[channelId] {
			continue
		}
		seen[channelId] = true
		if channel, ok := channelsIDM[channelId]; ok {
			candidates = append(candidates, channel)
		}
	}
	sortCascadeCandidates(group, candidates)
	return candidates, nil
}

func cascadeCandidatesFromDB(group string, model string, requestPath string) ([]*Channel, error) {
	normalizedModel := ratio_setting.FormatMatchingModelName(model)
	models := []string{model}
	if normalizedModel != model {
		models = append(models, normalizedModel)
	}

	var abilities []Ability
	err := DB.Where(commonGroupCol+" = ? and model in ? and enabled = ?", group, models, true).
		Order("priority DESC").Order("weight DESC").Find(&abilities).Error
	if err != nil {
		return nil, err
	}
	abilities = filterAbilitiesByRequestPathAndModel(abilities, requestPath, model)
	if len(abilities) == 0 {
		return nil, nil
	}

	seen := make(map[int]bool, len(abilities))
	channelIds := make([]int, 0, len(abilities))
	for _, ability := range abilities {
		if seen[ability.ChannelId] {
			continue
		}
		seen[ability.ChannelId] = true
		channelIds = append(channelIds, ability.ChannelId)
	}

	var channels []*Channel
	if err := DB.Where("id in ?", channelIds).Find(&channels).Error; err != nil {
		return nil, err
	}
	channelById := make(map[int]*Channel, len(channels))
	for _, channel := range channels {
		channelById[channel.Id] = channel
	}
	candidates := make([]*Channel, 0, len(channelIds))
	for _, channelId := range channelIds {
		if channel, ok := channelById[channelId]; ok {
			candidates = append(candidates, channel)
		}
	}
	sortCascadeCandidates(group, candidates)
	return candidates, nil
}
