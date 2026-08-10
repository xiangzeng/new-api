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
	require.NoError(t, db.AutoMigrate(&User{}, &TwoFA{}, &OpenApp{}, &OpenCredential{}))
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
// random value. Anything the open API signs must be verifiable across it.
func simulateProcessRestart(t *testing.T) {
	t.Helper()
	previous := common.SessionSecret
	common.SessionSecret = uuid.New().String()
	t.Cleanup(func() { common.SessionSecret = previous })
}

func TestOpenAppSecretSurvivesRestart(t *testing.T) {
	setupOpenApiTestDB(t)

	app, secret, err := CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)

	simulateProcessRestart(t)

	authenticated, err := ValidateOpenApp(app.AppId, secret, "203.0.113.7")
	require.NoError(t, err)
	assert.Equal(t, app.AppId, authenticated.AppId)
}

func TestValidateOpenAppRejectsWrongSecret(t *testing.T) {
	setupOpenApiTestDB(t)

	app, secret, err := CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(app.AppId, OpenAppIdPrefix))
	require.True(t, strings.HasPrefix(secret, OpenAppSecretPrefix))

	authenticated, err := ValidateOpenApp(app.AppId, secret, "203.0.113.7")
	require.NoError(t, err)
	assert.Equal(t, app.AppId, authenticated.AppId)

	_, err = ValidateOpenApp(app.AppId, secret+"x", "203.0.113.7")
	assert.ErrorIs(t, err, ErrOpenAppUnauthorized)

	// An unknown app id must be indistinguishable from a wrong secret so the
	// endpoint cannot be used to enumerate which partners exist.
	_, err = ValidateOpenApp("oapp_does_not_exist", secret, "203.0.113.7")
	assert.ErrorIs(t, err, ErrOpenAppUnauthorized)
}

func TestValidateOpenAppEnforcesStatusAndIpAllowList(t *testing.T) {
	setupOpenApiTestDB(t)

	app, secret, err := CreateOpenApp("Partner Site", "203.0.113.0/24\n198.51.100.9", 0)
	require.NoError(t, err)

	_, err = ValidateOpenApp(app.AppId, secret, "203.0.113.7")
	assert.NoError(t, err)
	_, err = ValidateOpenApp(app.AppId, secret, "198.51.100.9")
	assert.NoError(t, err)
	_, err = ValidateOpenApp(app.AppId, secret, "192.0.2.5")
	assert.ErrorIs(t, err, ErrOpenAppIpNotAllowed)

	_, err = UpdateOpenApp(app.Id, "Partner Site", "", OpenAppStatusDisabled, 0)
	require.NoError(t, err)
	_, err = ValidateOpenApp(app.AppId, secret, "203.0.113.7")
	assert.ErrorIs(t, err, ErrOpenAppDisabled)
}

func TestDisablingOpenAppRevokesIssuedCredentials(t *testing.T) {
	db := setupOpenApiTestDB(t)

	app, _, err := CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)

	token, _, err := IssueOpenCredential(user.Id, app.AppId, user.AuthVersion, "203.0.113.7", "")
	require.NoError(t, err)
	_, _, err = ValidateOpenCredential(token)
	require.NoError(t, err)

	_, err = UpdateOpenApp(app.Id, "Partner Site", "", OpenAppStatusDisabled, 0)
	require.NoError(t, err)

	_, _, err = ValidateOpenCredential(token)
	assert.ErrorIs(t, err, ErrOpenCredentialRevoked)
}

func TestResetOpenAppSecretRotatesAndRevokes(t *testing.T) {
	db := setupOpenApiTestDB(t)

	app, secret, err := CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	token, _, err := IssueOpenCredential(user.Id, app.AppId, user.AuthVersion, "203.0.113.7", "")
	require.NoError(t, err)

	rotated, newSecret, err := ResetOpenAppSecret(app.Id)
	require.NoError(t, err)
	assert.NotEqual(t, secret, newSecret)
	assert.NotEqual(t, app.SecretHint, rotated.SecretHint)

	_, err = ValidateOpenApp(app.AppId, secret, "203.0.113.7")
	assert.ErrorIs(t, err, ErrOpenAppUnauthorized)
	_, err = ValidateOpenApp(app.AppId, newSecret, "203.0.113.7")
	assert.NoError(t, err)

	// Rotating the secret must also cut off credentials minted under the old
	// one, otherwise a leaked secret keeps paying out through its tokens.
	_, _, err = ValidateOpenCredential(token)
	assert.ErrorIs(t, err, ErrOpenCredentialRevoked)
}

func TestOpenAppSecretHintDoesNotExposeSecret(t *testing.T) {
	setupOpenApiTestDB(t)

	app, secret, err := CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)

	assert.NotContains(t, app.SecretHint, strings.TrimPrefix(secret, OpenAppSecretPrefix)[:8])
	assert.True(t, strings.HasSuffix(app.SecretHint, secret[len(secret)-openAppHintLength:]))

	var stored OpenApp
	require.NoError(t, DB.Where("id = ?", app.Id).First(&stored).Error)
	assert.NotEqual(t, secret, stored.SecretHash)
	assert.NotContains(t, stored.SecretHash, secret)
}
