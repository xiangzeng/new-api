package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// 孤儿分组清理：分组从「分组倍率」配置里删掉后，渠道 group 字段与 abilities 表
// 仍残留该组名，编排页因此还能看到它。这里负责把组名从渠道上摘干净。
// 详见 docs/channel/cascade-failover.md

// PurgeGroupSkippedChannel 被跳过的渠道（该分组是它唯一的分组）
type PurgeGroupSkippedChannel struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

// PurgeGroupResult 孤儿分组清理结果
type PurgeGroupResult struct {
	Updated int                        `json:"updated"`
	Skipped []PurgeGroupSkippedChannel `json:"skipped"`
}

// PurgeChannelGroup 把指定分组从所有渠道的分组列表里摘掉，并重建这些渠道的 abilities。
// 只挂着这一个分组的渠道会被跳过——摘完 group 变空串，该渠道将失去全部 ability
// 成为不可路由的孤岛——改为在结果里返回，交管理员手工处理。
// 调用方负责刷新渠道缓存（InitChannelCache）。
func PurgeChannelGroup(group string) (*PurgeGroupResult, error) {
	group = strings.TrimSpace(group)
	if group == "" {
		return nil, errors.New("分组名为空")
	}
	channels, err := GetAllChannels(0, -1, false, false)
	if err != nil {
		return nil, err
	}

	result := &PurgeGroupResult{Skipped: make([]PurgeGroupSkippedChannel, 0)}
	targets := make([]*Channel, 0)
	for _, channel := range channels {
		remaining := make([]string, 0, len(channel.GetGroups()))
		hit := false
		for _, name := range channel.GetGroups() {
			if name == group {
				hit = true
				continue
			}
			remaining = append(remaining, name)
		}
		if !hit {
			continue
		}
		if len(remaining) == 0 {
			result.Skipped = append(result.Skipped, PurgeGroupSkippedChannel{
				Id:   channel.Id,
				Name: channel.Name,
			})
			continue
		}
		channel.Group = strings.Join(remaining, ",")
		targets = append(targets, channel)
	}
	if len(targets) == 0 {
		return result, nil
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		for _, channel := range targets {
			if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).
				Update("group", channel.Group).Error; err != nil {
				return err
			}
			// abilities 按 group×model 展开存储，改完 group 必须整体重建
			if err := channel.UpdateAbilities(tx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result.Updated = len(targets)
	return result, nil
}
