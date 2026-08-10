package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openResellerCenterForTest(t *testing.T, db *gorm.DB, userId int, receiveId string, createdAt int64) {
	t.Helper()
	require.NoError(t, db.Create(&ResellerProfile{
		UserId:          userId,
		Status:          ResellerStatusActive,
		ReceivePublicId: receiveId,
		PricingVersion:  1,
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}).Error)
}

func TestListResellerRosterCoversEveryResellerWithItsCustomerCount(t *testing.T) {
	db := setupResellerAdminTestDB(t)
	reseller := createResellerAdminUser(t, db, "roster-reseller")
	require.NoError(t, db.Model(&User{}).Where("id = ?", reseller.Id).
		Update("display_name", "Northern Depot").Error)
	idle := createResellerAdminUser(t, db, "roster-idle-reseller")
	plain := createResellerAdminUser(t, db, "roster-plain-user")
	firstCustomer := createResellerAdminUser(t, db, "roster-customer-one")
	secondCustomer := createResellerAdminUser(t, db, "roster-customer-two")

	_, err := AdminBindResellerCustomer(reseller.Id, "", firstCustomer.Id, 1_700_000_000)
	require.NoError(t, err)
	_, err = AdminBindResellerCustomer(reseller.Id, "", secondCustomer.Id, 1_700_000_100)
	require.NoError(t, err)
	// A center with no customers yet must still be listed, otherwise the operator
	// cannot find it to hand it its first customer.
	openResellerCenterForTest(t, db, idle.Id, "rcv-roster-idle", 1_700_000_200)

	items, total, err := ListResellerRoster("", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, items, 2)

	byUserId := make(map[int]ResellerRosterItem, len(items))
	for _, item := range items {
		byUserId[item.UserId] = item
	}
	require.Contains(t, byUserId, reseller.Id)
	require.Contains(t, byUserId, idle.Id)
	// Being someone's customer is not the same as running a center.
	assert.NotContains(t, byUserId, plain.Id)
	assert.NotContains(t, byUserId, firstCustomer.Id)

	assert.Equal(t, int64(2), byUserId[reseller.Id].CustomerCount)
	assert.Equal(t, int64(0), byUserId[idle.Id].CustomerCount)
	assert.Equal(t, "Northern Depot", byUserId[reseller.Id].DisplayName)
	assert.Equal(t, "roster-reseller", byUserId[reseller.Id].Username)
	assert.Equal(t, ResellerStatusActive, byUserId[reseller.Id].Status)
}

func TestListResellerRosterKeywordMatchesUsernameAndDisplayName(t *testing.T) {
	db := setupResellerAdminTestDB(t)
	named := createResellerAdminUser(t, db, "keyword-reseller-alpha")
	require.NoError(t, db.Model(&User{}).Where("id = ?", named.Id).
		Update("display_name", "Northern Depot").Error)
	other := createResellerAdminUser(t, db, "keyword-reseller-beta")
	openResellerCenterForTest(t, db, named.Id, "rcv-keyword-alpha", 1_700_000_000)
	openResellerCenterForTest(t, db, other.Id, "rcv-keyword-beta", 1_700_000_100)

	items, total, err := ListResellerRoster("Northern", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, named.Id, items[0].UserId)

	items, total, err = ListResellerRoster("beta", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, other.Id, items[0].UserId)

	_, total, err = ListResellerRoster("no-such-reseller", 0, 10)
	require.NoError(t, err)
	assert.Zero(t, total)
}

func TestListResellerRosterSkipsDeletedUsers(t *testing.T) {
	db := setupResellerAdminTestDB(t)
	gone := createResellerAdminUser(t, db, "roster-deleted-reseller")
	openResellerCenterForTest(t, db, gone.Id, "rcv-roster-deleted", 1_700_000_000)
	require.NoError(t, db.Delete(&User{}, gone.Id).Error)

	items, total, err := ListResellerRoster("", 0, 10)
	require.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, items)
}
