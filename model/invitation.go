package model

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type InvitationSummary struct {
	InviterID                int    `json:"inviter_id" gorm:"column:inviter_id"`
	InviterUsername          string `json:"inviter_username" gorm:"column:inviter_username"`
	InviterDisplayName       string `json:"inviter_display_name" gorm:"column:inviter_display_name"`
	InviterEmail             string `json:"inviter_email" gorm:"column:inviter_email"`
	InviterDeleted           bool   `json:"inviter_deleted" gorm:"column:inviter_deleted"`
	AffCode                  string `json:"aff_code" gorm:"column:aff_code"`
	AffQuota                 int64  `json:"aff_quota" gorm:"column:aff_quota"`
	AffHistoryQuota          int64  `json:"aff_history_quota" gorm:"column:aff_history_quota"`
	InviteeCount             int64  `json:"invitee_count" gorm:"column:invitee_count"`
	InviteeTotalUsedQuota    int64  `json:"invitee_total_used_quota" gorm:"column:invitee_total_used_quota"`
	InviteeTotalRequestCount int64  `json:"invitee_total_request_count" gorm:"column:invitee_total_request_count"`
	PeriodQuota              int64  `json:"period_quota" gorm:"-"`
	PeriodRequestCount       int64  `json:"period_request_count" gorm:"-"`
	PeriodPromptTokens       int64  `json:"period_prompt_tokens" gorm:"-"`
	PeriodCompletionTokens   int64  `json:"period_completion_tokens" gorm:"-"`
}

type InvitationInvitee struct {
	InviteeID              int    `json:"invitee_id" gorm:"column:invitee_id"`
	Username               string `json:"username" gorm:"column:username"`
	DisplayName            string `json:"display_name" gorm:"column:display_name"`
	Email                  string `json:"email" gorm:"column:email"`
	Group                  string `json:"group" gorm:"column:user_group"`
	Status                 int    `json:"status" gorm:"column:status"`
	IsDeleted              bool   `json:"is_deleted" gorm:"column:is_deleted"`
	Quota                  int64  `json:"quota" gorm:"column:quota"`
	UsedQuota              int64  `json:"used_quota" gorm:"column:used_quota"`
	RequestCount           int64  `json:"request_count" gorm:"column:request_count"`
	PeriodQuota            int64  `json:"period_quota" gorm:"-"`
	PeriodRequestCount     int64  `json:"period_request_count" gorm:"-"`
	PeriodPromptTokens     int64  `json:"period_prompt_tokens" gorm:"-"`
	PeriodCompletionTokens int64  `json:"period_completion_tokens" gorm:"-"`
}

type invitationUserLink struct {
	InviteeID int `gorm:"column:invitee_id"`
	InviterID int `gorm:"column:inviter_id"`
}

type invitationUsageStat struct {
	UserID           int   `gorm:"column:user_id"`
	Quota            int64 `gorm:"column:quota"`
	RequestCount     int64 `gorm:"column:request_count"`
	PromptTokens     int64 `gorm:"column:prompt_tokens"`
	CompletionTokens int64 `gorm:"column:completion_tokens"`
}

const invitationUsageStatsBatchSize = 500

func invitationContainsPattern(keyword string) string {
	keyword = strings.TrimSpace(keyword)
	keyword = strings.ReplaceAll(keyword, "!", "!!")
	keyword = strings.ReplaceAll(keyword, "%", "!%")
	keyword = strings.ReplaceAll(keyword, "_", "!_")
	return "%" + keyword + "%"
}

func applyInvitationSummaryKeyword(query *gorm.DB, keyword string) *gorm.DB {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return query
	}
	pattern := invitationContainsPattern(keyword)
	if id, err := strconv.Atoi(keyword); err == nil {
		return query.Where(
			"(inviter.id = ? OR inviter.username LIKE ? ESCAPE '!' OR inviter.display_name LIKE ? ESCAPE '!' OR inviter.email LIKE ? ESCAPE '!' OR inviter.aff_code LIKE ? ESCAPE '!')",
			id,
			pattern,
			pattern,
			pattern,
			pattern,
		)
	}
	return query.Where(
		"(inviter.username LIKE ? ESCAPE '!' OR inviter.display_name LIKE ? ESCAPE '!' OR inviter.email LIKE ? ESCAPE '!' OR inviter.aff_code LIKE ? ESCAPE '!')",
		pattern,
		pattern,
		pattern,
		pattern,
	)
}

func applyInvitationInviteeKeyword(query *gorm.DB, keyword string) *gorm.DB {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return query
	}
	pattern := invitationContainsPattern(keyword)
	if id, err := strconv.Atoi(keyword); err == nil {
		return query.Where(
			"(invitee.id = ? OR invitee.username LIKE ? ESCAPE '!' OR invitee.display_name LIKE ? ESCAPE '!' OR invitee.email LIKE ? ESCAPE '!')",
			id,
			pattern,
			pattern,
			pattern,
		)
	}
	return query.Where(
		"(invitee.username LIKE ? ESCAPE '!' OR invitee.display_name LIKE ? ESCAPE '!' OR invitee.email LIKE ? ESCAPE '!')",
		pattern,
		pattern,
		pattern,
	)
}

func invitationSummaryBaseQuery() *gorm.DB {
	return DB.Table("users AS invitee").
		Joins("JOIN users AS inviter ON inviter.id = invitee.inviter_id").
		Where("invitee.inviter_id <> ?", 0)
}

func getInvitationUsageStats(userIDs []int, startTimestamp int64, endTimestamp int64) (map[int]invitationUsageStat, error) {
	statsByUser := make(map[int]invitationUsageStat)
	if len(userIDs) == 0 {
		return statsByUser, nil
	}

	for start := 0; start < len(userIDs); start += invitationUsageStatsBatchSize {
		end := start + invitationUsageStatsBatchSize
		if end > len(userIDs) {
			end = len(userIDs)
		}

		var stats []invitationUsageStat
		query := LOG_DB.Table("logs").
			Select("user_id, COALESCE(SUM(quota), 0) AS quota, COUNT(*) AS request_count, COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens, COALESCE(SUM(completion_tokens), 0) AS completion_tokens").
			Where("type = ?", LogTypeConsume).
			Where("user_id IN ?", userIDs[start:end])
		if startTimestamp > 0 {
			query = query.Where("created_at >= ?", startTimestamp)
		}
		if endTimestamp > 0 {
			query = query.Where("created_at <= ?", endTimestamp)
		}
		if err := query.Group("user_id").Scan(&stats).Error; err != nil {
			return statsByUser, err
		}
		for _, stat := range stats {
			statsByUser[stat.UserID] = stat
		}
	}
	return statsByUser, nil
}

func appendInvitationPeriodStats(summaries []*InvitationSummary, startTimestamp int64, endTimestamp int64) error {
	if len(summaries) == 0 {
		return nil
	}

	inviterIDs := make([]int, 0, len(summaries))
	summaryByInviter := make(map[int]*InvitationSummary, len(summaries))
	for _, summary := range summaries {
		inviterIDs = append(inviterIDs, summary.InviterID)
		summaryByInviter[summary.InviterID] = summary
	}

	var links []invitationUserLink
	if err := DB.Table("users").
		Select("id AS invitee_id, inviter_id").
		Where("inviter_id IN ?", inviterIDs).
		Find(&links).Error; err != nil {
		return err
	}

	inviteeIDs := make([]int, 0, len(links))
	for _, link := range links {
		inviteeIDs = append(inviteeIDs, link.InviteeID)
	}
	statsByUser, err := getInvitationUsageStats(inviteeIDs, startTimestamp, endTimestamp)
	if err != nil {
		return err
	}

	for _, link := range links {
		stat, ok := statsByUser[link.InviteeID]
		if !ok {
			continue
		}
		summary := summaryByInviter[link.InviterID]
		if summary == nil {
			continue
		}
		summary.PeriodQuota += stat.Quota
		summary.PeriodRequestCount += stat.RequestCount
		summary.PeriodPromptTokens += stat.PromptTokens
		summary.PeriodCompletionTokens += stat.CompletionTokens
	}
	return nil
}

func GetInvitationSummaries(pageInfo *common.PageInfo, keyword string, startTimestamp int64, endTimestamp int64) (summaries []*InvitationSummary, total int64, err error) {
	countQuery := applyInvitationSummaryKeyword(invitationSummaryBaseQuery(), keyword)
	if err = countQuery.Distinct("invitee.inviter_id").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	dataQuery := applyInvitationSummaryKeyword(invitationSummaryBaseQuery(), keyword)
	err = dataQuery.Select(
		"inviter.id AS inviter_id, inviter.username AS inviter_username, inviter.display_name AS inviter_display_name, inviter.email AS inviter_email, CASE WHEN inviter.deleted_at IS NULL THEN 0 ELSE 1 END AS inviter_deleted, inviter.aff_code AS aff_code, COALESCE(inviter.aff_quota, 0) AS aff_quota, COALESCE(inviter.aff_history, 0) AS aff_history_quota, COUNT(invitee.id) AS invitee_count, COALESCE(SUM(invitee.used_quota), 0) AS invitee_total_used_quota, COALESCE(SUM(invitee.request_count), 0) AS invitee_total_request_count",
	).Group(
		"inviter.id, inviter.username, inviter.display_name, inviter.email, inviter.deleted_at, inviter.aff_code, inviter.aff_quota, inviter.aff_history",
	).Order("invitee_count DESC, inviter.id DESC").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Scan(&summaries).Error
	if err != nil {
		return nil, 0, err
	}

	if err = appendInvitationPeriodStats(summaries, startTimestamp, endTimestamp); err != nil {
		return nil, 0, err
	}
	return summaries, total, nil
}

func GetInvitationInvitees(inviterID int, pageInfo *common.PageInfo, keyword string, startTimestamp int64, endTimestamp int64) (invitees []*InvitationInvitee, total int64, err error) {
	countQuery := applyInvitationInviteeKeyword(
		DB.Table("users AS invitee").Where("invitee.inviter_id = ?", inviterID),
		keyword,
	)
	if err = countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	dataQuery := applyInvitationInviteeKeyword(
		DB.Table("users AS invitee").Where("invitee.inviter_id = ?", inviterID),
		keyword,
	)
	err = dataQuery.Select(
		"invitee.id AS invitee_id, invitee.username, invitee.display_name, invitee.email, invitee." + commonGroupCol + " AS user_group, invitee.status, CASE WHEN invitee.deleted_at IS NULL THEN 0 ELSE 1 END AS is_deleted, COALESCE(invitee.quota, 0) AS quota, COALESCE(invitee.used_quota, 0) AS used_quota, COALESCE(invitee.request_count, 0) AS request_count",
	).Order("invitee.id DESC").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Scan(&invitees).Error
	if err != nil {
		return nil, 0, err
	}

	inviteeIDs := make([]int, 0, len(invitees))
	inviteeByID := make(map[int]*InvitationInvitee, len(invitees))
	for _, invitee := range invitees {
		inviteeIDs = append(inviteeIDs, invitee.InviteeID)
		inviteeByID[invitee.InviteeID] = invitee
	}
	statsByUser, err := getInvitationUsageStats(inviteeIDs, startTimestamp, endTimestamp)
	if err != nil {
		return nil, 0, err
	}
	for userID, stat := range statsByUser {
		invitee := inviteeByID[userID]
		if invitee == nil {
			continue
		}
		invitee.PeriodQuota = stat.Quota
		invitee.PeriodRequestCount = stat.RequestCount
		invitee.PeriodPromptTokens = stat.PromptTokens
		invitee.PeriodCompletionTokens = stat.CompletionTokens
	}

	return invitees, total, nil
}
