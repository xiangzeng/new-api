package model

import (
	"strings"
)

// ResellerRosterItem is the admin console view of one user running a reseller
// center. Commission balances are read straight off the profile so the operator
// sees the same numbers the reseller sees in its own center, and UserStatus is
// carried separately from the profile Status because a disabled account can
// still own an active profile.
type ResellerRosterItem struct {
	UserId                   int    `json:"user_id"`
	Username                 string `json:"username"`
	DisplayName              string `json:"display_name"`
	UserStatus               int    `json:"user_status"`
	Status                   string `json:"status"`
	CustomerCount            int64  `json:"customer_count"`
	PendingCommissionQuota   int64  `json:"pending_commission_quota"`
	AvailableCommissionQuota int64  `json:"available_commission_quota"`
	CreatedAt                int64  `json:"created_at"`
}

// ListResellerRoster pages through every user that has opened a reseller
// center. There is no such thing as a "reseller" flag on the user row — the
// existence of a profile is what makes one — so the roster is driven by
// reseller_profiles and joined back to users for the human-readable identity.
//
// keyword matches username or display name, because an operator asked to move a
// customer under "some reseller" knows the name, not the id.
func ListResellerRoster(keyword string, offset int, limit int) ([]ResellerRosterItem, int64, error) {
	query := DB.Model(&ResellerProfile{}).
		Joins("JOIN users ON users.id = reseller_profiles.user_id AND users.deleted_at IS NULL")
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("users.username LIKE ? OR users.display_name LIKE ?", like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]ResellerRosterItem, 0)
	if total == 0 {
		return items, 0, nil
	}

	err := query.
		Select(`reseller_profiles.user_id AS user_id,
			users.username AS username,
			users.display_name AS display_name,
			users.status AS user_status,
			reseller_profiles.status AS status,
			reseller_profiles.pending_commission_quota AS pending_commission_quota,
			reseller_profiles.available_commission_quota AS available_commission_quota,
			reseller_profiles.created_at AS created_at`).
		Order("reseller_profiles.created_at DESC, reseller_profiles.id DESC").
		Offset(offset).Limit(limit).
		Scan(&items).Error
	if err != nil {
		return nil, 0, err
	}
	if len(items) == 0 {
		return items, total, nil
	}

	resellerIds := make([]int, 0, len(items))
	for _, item := range items {
		resellerIds = append(resellerIds, item.UserId)
	}
	var counts []struct {
		ResellerId int   `gorm:"column:reseller_id"`
		Total      int64 `gorm:"column:total"`
	}
	if err := DB.Model(&ResellerCustomer{}).
		Select("reseller_id, COUNT(*) AS total").
		Where("reseller_id IN ?", resellerIds).
		Group("reseller_id").
		Scan(&counts).Error; err != nil {
		return nil, 0, err
	}
	totalByReseller := make(map[int]int64, len(counts))
	for _, row := range counts {
		totalByReseller[row.ResellerId] = row.Total
	}
	for i := range items {
		items[i].CustomerCount = totalByReseller[items[i].UserId]
	}
	return items, total, nil
}
