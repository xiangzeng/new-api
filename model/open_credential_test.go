package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOpenApiTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousType := common.MainDatabaseType()
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &OpenCredential{}))
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		common.RedisEnabled = previousRedis
		common.SetMainDatabaseType(previousType)
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func createOpenApiTestUser(t *testing.T, db *gorm.DB, username string, password string, status int) User {
	t.Helper()
	hash, err := common.Password2Hash(password)
	require.NoError(t, err)
	user := User{
		Username:    username,
		Password:    hash,
		DisplayName: username,
		AffCode:     "aff-" + username,
		Status:      status,
		Quota:       500000,
		UsedQuota:   123456,
		AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

// simulateProcessRestart reproduces what a redeploy does to a New API instance
// that has no SESSION_SECRET configured: common.SessionSecret gets a brand new
// random value. Anything the balance API signs must be verifiable across it.
func simulateProcessRestart(t *testing.T) {
	t.Helper()
	previous := common.SessionSecret
	common.SessionSecret = uuid.New().String()
	t.Cleanup(func() { common.SessionSecret = previous })
}

// A balance key is advertised as long lived: it ends only when its owner
// revokes it or the account is disabled. A redeploy is neither, so it must not
// be able to end one either.
func TestOpenCredentialSurvivesRestart(t *testing.T) {
	db := setupOpenApiTestDB(t)

	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	token, _, err := IssueOpenCredential(user.Id, "laptop script", "203.0.113.7")
	require.NoError(t, err)

	simulateProcessRestart(t)

	credential, owner, err := ValidateOpenCredential(token)
	require.NoError(t, err)
	assert.Equal(t, user.Id, owner.Id)
	assert.Equal(t, OpenScopeBalanceRead, credential.Scope)

	// Revocation must still resolve the same row after the restart, otherwise a
	// key could survive the very action meant to end it.
	require.NoError(t, RevokeOpenCredentialByToken(token))
	_, _, err = ValidateOpenCredential(token)
	assert.ErrorIs(t, err, ErrOpenCredentialRevoked)
}

// The key belongs to its owner rather than to a third party, so it follows
// relay API keys and outlives a password change. Only revocation or losing the
// account ends it.
func TestValidateOpenCredentialSurvivesPasswordChange(t *testing.T) {
	db := setupOpenApiTestDB(t)

	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	token, _, err := IssueOpenCredential(user.Id, "laptop script", "203.0.113.7")
	require.NoError(t, err)

	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).
		Update("auth_version", user.AuthVersion+1).Error)

	_, owner, err := ValidateOpenCredential(token)
	require.NoError(t, err)
	assert.Equal(t, user.Id, owner.Id)
}

func TestIssueOpenCredentialKeepsEarlierKeysUntilTheLimit(t *testing.T) {
	db := setupOpenApiTestDB(t)

	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)

	tokens := make([]string, 0, OpenCredentialMaxPerUser)
	for i := range OpenCredentialMaxPerUser {
		token, _, err := IssueOpenCredential(user.Id, fmt.Sprintf("program %d", i), "203.0.113.7")
		require.NoError(t, err)
		tokens = append(tokens, token)
	}

	// One key per program is the point of the feature, so issuing a second one
	// must not retire the first.
	for _, token := range tokens {
		_, _, err := ValidateOpenCredential(token)
		assert.NoError(t, err)
	}

	_, _, err := IssueOpenCredential(user.Id, "one too many", "203.0.113.7")
	assert.ErrorIs(t, err, ErrOpenCredentialLimitReached)

	// Revoking frees a slot: the ceiling counts active keys, not issued ones.
	require.NoError(t, RevokeOpenCredentialByToken(tokens[0]))
	_, _, err = IssueOpenCredential(user.Id, "replacement", "203.0.113.7")
	assert.NoError(t, err)
}

func TestIssueOpenCredentialNormalizesName(t *testing.T) {
	db := setupOpenApiTestDB(t)

	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)

	_, credential, err := IssueOpenCredential(user.Id, "   ", "203.0.113.7")
	require.NoError(t, err)
	assert.Equal(t, openCredentialDefaultName, credential.Name)

	_, _, err = IssueOpenCredential(user.Id, strings.Repeat("名", openCredentialNameMaxLength+1), "203.0.113.7")
	assert.ErrorIs(t, err, ErrOpenCredentialNameTooLong)

	// The bound counts characters, not bytes: a 50-character CJK name is
	// legitimate and must not be rejected for its UTF-8 length.
	_, credential, err = IssueOpenCredential(user.Id, strings.Repeat("名", openCredentialNameMaxLength), "203.0.113.7")
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("名", openCredentialNameMaxLength), credential.Name)
}

func TestValidateOpenCredentialRejectsDisabledOwner(t *testing.T) {
	db := setupOpenApiTestDB(t)

	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	token, _, err := IssueOpenCredential(user.Id, "laptop script", "203.0.113.7")
	require.NoError(t, err)

	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).
		Update("status", common.UserStatusDisabled).Error)

	_, _, err = ValidateOpenCredential(token)
	assert.ErrorIs(t, err, ErrOpenCredentialUserOff)
}

func TestValidateOpenCredentialRejectsUnknownToken(t *testing.T) {
	setupOpenApiTestDB(t)

	_, _, err := ValidateOpenCredential("obk_not_a_real_credential")
	assert.ErrorIs(t, err, ErrOpenCredentialInvalid)
	_, _, err = ValidateOpenCredential("")
	assert.ErrorIs(t, err, ErrOpenCredentialInvalid)
}

func TestRevokeOpenCredentialByUserIsScopedToOwner(t *testing.T) {
	db := setupOpenApiTestDB(t)

	owner := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	stranger := createOpenApiTestUser(t, db, "bob", "battery-staple", common.UserStatusEnabled)

	token, credential, err := IssueOpenCredential(owner.Id, "laptop script", "203.0.113.7")
	require.NoError(t, err)

	// Guessing a row id must not let one account end another account's key.
	assert.ErrorIs(t, RevokeOpenCredentialByUser(stranger.Id, credential.Id), ErrOpenCredentialInvalid)
	_, _, err = ValidateOpenCredential(token)
	assert.NoError(t, err)

	require.NoError(t, RevokeOpenCredentialByUser(owner.Id, credential.Id))
	_, _, err = ValidateOpenCredential(token)
	assert.ErrorIs(t, err, ErrOpenCredentialRevoked)
}

func TestRevokeOpenCredentialByTokenIsIdempotentlyRejected(t *testing.T) {
	db := setupOpenApiTestDB(t)

	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	token, _, err := IssueOpenCredential(user.Id, "laptop script", "203.0.113.7")
	require.NoError(t, err)

	require.NoError(t, RevokeOpenCredentialByToken(token))
	assert.ErrorIs(t, RevokeOpenCredentialByToken(token), ErrOpenCredentialInvalid)
}

func TestListOpenCredentialsByUserShowsOnlyActiveKeys(t *testing.T) {
	db := setupOpenApiTestDB(t)

	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)

	_, revoked, err := IssueOpenCredential(user.Id, "retired script", "203.0.113.7")
	require.NoError(t, err)
	_, _, err = IssueOpenCredential(user.Id, "phone widget", "203.0.113.8")
	require.NoError(t, err)
	require.NoError(t, RevokeOpenCredentialByUser(user.Id, revoked.Id))

	items, err := ListOpenCredentialsByUser(user.Id)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "phone widget", items[0].Name)
	assert.Equal(t, OpenScopeBalanceRead, items[0].Scope)
	assert.Contains(t, items[0].TokenHint, OpenCredentialPrefix)
}

func TestGetOpenBalanceSnapshotReadsWalletColumns(t *testing.T) {
	db := setupOpenApiTestDB(t)

	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).
		Update("request_count", 831).Error)

	snapshot, err := GetOpenBalanceSnapshot(user.Id)
	require.NoError(t, err)
	assert.Equal(t, "alice", snapshot.Username)
	assert.Equal(t, 500000, snapshot.Quota)
	assert.Equal(t, 123456, snapshot.UsedQuota)
	assert.Equal(t, 831, snapshot.RequestCount)

	_, err = GetOpenBalanceSnapshot(user.Id + 999)
	assert.ErrorIs(t, err, ErrOpenCredentialInvalid)
}
