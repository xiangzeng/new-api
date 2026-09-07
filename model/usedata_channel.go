package model

import "sort"

// 渠道维度的用户消耗排名：回答「这条渠道的钱是谁花的」。
// 数据源是数据看板小时桶 quota_data（与渠道分日用量同一条消费日志路径写入），
// 按 channel_id 索引扫描后按 用户 × 分组 聚合，生产上 3 天范围毫秒级返回。
// 分组维度保留下来是为了给每个用户标出「走这个分组时的计费倍率」（千人千面），
// 倍率本身由 service 层按当前定价配置填充，这里只出数量。

// ChannelUserGroupUsage 用户在某分组上的消耗明细及当前生效倍率
type ChannelUserGroupUsage struct {
	Group     string `json:"group"`
	Quota     int64  `json:"quota"`
	Count     int64  `json:"count"`
	TokenUsed int64  `json:"token_used"`
	// 以下由 service.FillChannelUserUsageRatios 填充：Ratio 为最终生效倍率（含站长折扣），
	// BaseRatio 为折扣前倍率，DefaultRatio 为分组默认倍率，RatioSource 标明 BaseRatio 的来源
	Ratio              float64 `json:"ratio"`
	BaseRatio          float64 `json:"base_ratio"`
	DefaultRatio       float64 `json:"default_ratio"`
	RatioSource        string  `json:"ratio_source"`
	ResellerMultiplier float64 `json:"reseller_multiplier"`
}

// ChannelUserUsage 单个用户在某渠道、某时间范围内的汇总消耗（Groups 按消耗降序）
type ChannelUserUsage struct {
	UserID    int                      `json:"user_id"`
	Username  string                   `json:"username"`
	Quota     int64                    `json:"quota"`
	Count     int64                    `json:"count"`
	TokenUsed int64                    `json:"token_used"`
	Groups    []*ChannelUserGroupUsage `json:"groups"`
}

func (u *ChannelUserUsage) add(group string, quota, count, tokenUsed int64) {
	u.Quota += quota
	u.Count += count
	u.TokenUsed += tokenUsed
	for _, item := range u.Groups {
		if item.Group == group {
			item.Quota += quota
			item.Count += count
			item.TokenUsed += tokenUsed
			return
		}
	}
	u.Groups = append(u.Groups, &ChannelUserGroupUsage{Group: group, Quota: quota, Count: count, TokenUsed: tokenUsed})
}

type channelUserGroupRow struct {
	UserID    int    `gorm:"column:user_id"`
	Username  string `gorm:"column:username"`
	UseGroup  string `gorm:"column:use_group"`
	Quota     int64  `gorm:"column:quota"`
	Count     int64  `gorm:"column:count"`
	TokenUsed int64  `gorm:"column:token_used"`
}

// GetChannelUserUsageRanking 返回渠道在 [startTime, endTime]（unix 秒，含边界）内按用户汇总的消耗，
// 额度降序，最多 limit 行（<= 0 不限）。尚未落库的内存增量一并合并，避免「今天」慢一拍。
func GetChannelUserUsageRanking(channelId int, startTime int64, endTime int64, limit int) ([]*ChannelUserUsage, error) {
	groupRows := make([]channelUserGroupRow, 0)
	err := DB.Table("quota_data").
		Select("user_id, username, use_group, sum(quota) as quota, sum(count) as count, sum(token_used) as token_used").
		Where("channel_id = ? and created_at >= ? and created_at <= ?", channelId, startTime, endTime).
		Group("user_id, username, use_group").
		Find(&groupRows).Error
	if err != nil {
		return nil, err
	}

	rows := make([]*ChannelUserUsage, 0, len(groupRows))
	byUser := make(map[int]*ChannelUserUsage, len(groupRows))
	for _, item := range groupRows {
		row, ok := byUser[item.UserID]
		if !ok {
			row = &ChannelUserUsage{UserID: item.UserID, Username: item.Username}
			byUser[item.UserID] = row
			rows = append(rows, row)
		}
		row.add(item.UseGroup, item.Quota, item.Count, item.TokenUsed)
	}

	CacheQuotaDataLock.Lock()
	for _, pending := range CacheQuotaData {
		if pending.ChannelID != channelId || pending.CreatedAt < startTime || pending.CreatedAt > endTime {
			continue
		}
		row, ok := byUser[pending.UserID]
		if !ok {
			row = &ChannelUserUsage{UserID: pending.UserID, Username: pending.Username}
			byUser[pending.UserID] = row
			rows = append(rows, row)
		}
		row.add(pending.UseGroup, int64(pending.Quota), int64(pending.Count), int64(pending.TokenUsed))
	}
	CacheQuotaDataLock.Unlock()

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Quota != rows[j].Quota {
			return rows[i].Quota > rows[j].Quota
		}
		return rows[i].UserID < rows[j].UserID
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	for _, row := range rows {
		sort.SliceStable(row.Groups, func(i, j int) bool {
			if row.Groups[i].Quota != row.Groups[j].Quota {
				return row.Groups[i].Quota > row.Groups[j].Quota
			}
			return row.Groups[i].Group < row.Groups[j].Group
		})
	}
	return rows, nil
}
