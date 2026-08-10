package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A credential is advertised to partners as long lived: it ends only when the
// user changes their password, someone revokes it, or the app is disabled. A
// redeploy is none of those, so it must not be able to end one either.
func TestOpenCredentialSurvivesRestart(t *testing.T) {
	db := setupOpenApiTestDB(t)

	app, _, err := CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	token, _, err := IssueOpenCredential(user.Id, app.AppId, user.AuthVersion, "203.0.113.7", "")
	require.NoError(t, err)

	simulateProcessRestart(t)

	credential, owner, err := ValidateOpenCredential(token)
	require.NoError(t, err)
	assert.Equal(t, user.Id, owner.Id)
	assert.Equal(t, OpenScopeBalanceRead, credential.Scope)

	// Revocation must still resolve the same row after the restart, otherwise a
	// credential could survive the very actions meant to end it.
	require.NoError(t, RevokeOpenCredentialByToken(token))
	_, _, err = ValidateOpenCredential(token)
	assert.ErrorIs(t, err, ErrOpenCredentialRevoked)
}

func TestIssueOpenCredentialReplacesPreviousGrant(t *testing.T) {
	db := setupOpenApiTestDB(t)

	app, _, err := CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)

	first, _, err := IssueOpenCredential(user.Id, app.AppId, user.AuthVersion, "203.0.113.7", "1.2.3.4")
	require.NoError(t, err)
	second, _, err := IssueOpenCredential(user.Id, app.AppId, user.AuthVersion, "203.0.113.7", "1.2.3.4")
	require.NoError(t, err)
	assert.NotEqual(t, first, second)

	// The clear-text credential is never stored, so re-exchanging cannot return
	// the old one. Leaving it active would accumulate a live token per login.
	_, _, err = ValidateOpenCredential(first)
	assert.ErrorIs(t, err, ErrOpenCredentialRevoked)
	_, _, err = ValidateOpenCredential(second)
	assert.NoError(t, err)

	var activeCount int64
	require.NoError(t, DB.Model(&OpenCredential{}).
		Where("user_id = ? AND app_id = ? AND status = ?", user.Id, app.AppId, OpenCredentialStatusActive).
		Count(&activeCount).Error)
	assert.Equal(t, int64(1), activeCount)
}

func TestIssueOpenCredentialKeepsOtherPartnersUntouched(t *testing.T) {
	db := setupOpenApiTestDB(t)

	first, _, err := CreateOpenApp("Partner One", "", 0)
	require.NoError(t, err)
	second, _, err := CreateOpenApp("Partner Two", "", 0)
	require.NoError(t, err)
	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)

	firstToken, _, err := IssueOpenCredential(user.Id, first.AppId, user.AuthVersion, "203.0.113.7", "")
	require.NoError(t, err)
	_, _, err = IssueOpenCredential(user.Id, second.AppId, user.AuthVersion, "203.0.113.8", "")
	require.NoError(t, err)

	_, _, err = ValidateOpenCredential(firstToken)
	assert.NoError(t, err)
}

func TestValidateOpenCredentialRejectsStaleAuthVersion(t *testing.T) {
	db := setupOpenApiTestDB(t)

	app, _, err := CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)

	token, _, err := IssueOpenCredential(user.Id, app.AppId, user.AuthVersion, "203.0.113.7", "")
	require.NoError(t, err)
	_, _, err = ValidateOpenCredential(token)
	require.NoError(t, err)

	// A password change bumps auth_version. Long-lived credentials must not
	// survive it, which is the whole reason the version is pinned on the row.
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).
		Update("auth_version", user.AuthVersion+1).Error)

	_, _, err = ValidateOpenCredential(token)
	assert.ErrorIs(t, err, ErrOpenCredentialRevoked)
}

func TestValidateOpenCredentialRejectsDisabledOwner(t *testing.T) {
	db := setupOpenApiTestDB(t)

	app, _, err := CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	token, _, err := IssueOpenCredential(user.Id, app.AppId, user.AuthVersion, "203.0.113.7", "")
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

	app, _, err := CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	owner := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	stranger := createOpenApiTestUser(t, db, "bob", "battery-staple", common.UserStatusEnabled)

	token, credential, err := IssueOpenCredential(owner.Id, app.AppId, owner.AuthVersion, "203.0.113.7", "")
	require.NoError(t, err)

	// Guessing a row id must not let one account end another account's grant.
	assert.ErrorIs(t, RevokeOpenCredentialByUser(stranger.Id, credential.Id), ErrOpenCredentialInvalid)
	_, _, err = ValidateOpenCredential(token)
	assert.NoError(t, err)

	require.NoError(t, RevokeOpenCredentialByUser(owner.Id, credential.Id))
	_, _, err = ValidateOpenCredential(token)
	assert.ErrorIs(t, err, ErrOpenCredentialRevoked)
}

func TestRevokeOpenCredentialByTokenIsIdempotentlyRejected(t *testing.T) {
	db := setupOpenApiTestDB(t)

	app, _, err := CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	token, _, err := IssueOpenCredential(user.Id, app.AppId, user.AuthVersion, "203.0.113.7", "")
	require.NoError(t, err)

	require.NoError(t, RevokeOpenCredentialByToken(token))
	assert.ErrorIs(t, RevokeOpenCredentialByToken(token), ErrOpenCredentialInvalid)
}

func TestListOpenCredentialsByUserShowsOnlyActiveGrants(t *testing.T) {
	db := setupOpenApiTestDB(t)

	first, _, err := CreateOpenApp("Partner One", "", 0)
	require.NoError(t, err)
	second, _, err := CreateOpenApp("Partner Two", "", 0)
	require.NoError(t, err)
	user := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)

	_, firstCredential, err := IssueOpenCredential(user.Id, first.AppId, user.AuthVersion, "203.0.113.7", "")
	require.NoError(t, err)
	_, _, err = IssueOpenCredential(user.Id, second.AppId, user.AuthVersion, "203.0.113.8", "")
	require.NoError(t, err)
	require.NoError(t, RevokeOpenCredentialByUser(user.Id, firstCredential.Id))

	items, err := ListOpenCredentialsByUser(user.Id)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, second.AppId, items[0].AppId)
	assert.Equal(t, "Partner Two", items[0].AppName)
	assert.Equal(t, OpenScopeBalanceRead, items[0].Scope)
}

func TestAuthenticateOpenApiUserDistinguishesDisabledAccount(t *testing.T) {
	db := setupOpenApiTestDB(t)

	enabled := createOpenApiTestUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	createOpenApiTestUser(t, db, "carol", "tr0ub4dor", common.UserStatusDisabled)

	authenticated, err := AuthenticateOpenApiUser("alice", "correct-horse")
	require.NoError(t, err)
	assert.Equal(t, enabled.Id, authenticated.Id)

	_, err = AuthenticateOpenApiUser("alice", "wrong-password")
	assert.ErrorIs(t, err, ErrInvalidCredentials)

	_, err = AuthenticateOpenApiUser("nobody", "correct-horse")
	assert.ErrorIs(t, err, ErrInvalidCredentials)

	// A correct password on a banned account reports the real reason: the
	// caller already proved knowledge of the secret, so nothing new leaks and
	// the partner can show an accurate message instead of "wrong password".
	_, err = AuthenticateOpenApiUser("carol", "tr0ub4dor")
	assert.ErrorIs(t, err, ErrOpenCredentialUserOff)

	// A wrong password on a banned account must still look like a bad password.
	_, err = AuthenticateOpenApiUser("carol", "wrong-password")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
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
