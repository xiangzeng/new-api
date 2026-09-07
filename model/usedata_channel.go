package model

import "sort"

// 渠道维度的用户消耗排名：回答「这条渠道的钱是谁花的」。
// 数据源是数据看板小时桶 quota_data（与渠道分日用量同一条消费日志路径写入），
// 按 channel_id 索引扫描后按用户聚合，生产上 3 天范围毫秒级返回。

// ChannelUserUsage 单个用户在某渠道、某时间范围内的汇总消耗
type ChannelUserUsage struct {
	UserID    int    `json:"user_id" gorm:"column:user_id"`
	Username  string `json:"username" gorm:"column:username"`
	Quota     int64  `json:"quota" gorm:"column:quota"`
	Count     int64  `json:"count" gorm:"column:count"`
	TokenUsed int64  `json:"token_used" gorm:"column:token_used"`
}

// GetChannelUserUsageRanking 返回渠道在 [startTime, endTime]（unix 秒，含边界）内按用户汇总的消耗，
// 额度降序，最多 limit 行（<= 0 不限）。尚未落库的内存增量一并合并，避免「今天」慢一拍。
func GetChannelUserUsageRanking(channelId int, startTime int64, endTime int64, limit int) ([]*ChannelUserUsage, error) {
	rows := make([]*ChannelUserUsage, 0)
	err := DB.Table("quota_data").
		Select("user_id, username, sum(quota) as quota, sum(count) as count, sum(token_used) as token_used").
		Where("channel_id = ? and created_at >= ? and created_at <= ?", channelId, startTime, endTime).
		Group("user_id, username").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	byUser := make(map[int]*ChannelUserUsage, len(rows))
	for _, row := range rows {
		byUser[row.UserID] = row
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
		row.Quota += int64(pending.Quota)
		row.Count += int64(pending.Count)
		row.TokenUsed += int64(pending.TokenUsed)
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
	return rows, nil
}
