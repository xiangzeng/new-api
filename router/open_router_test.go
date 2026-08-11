package router

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupOpenBalanceApi wires the real SetOpenRouter chain against an in-memory
// database so the tests assert the shipped middleware order, not a hand-built
// approximation of it.
func setupOpenBalanceApi(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousType := common.MainDatabaseType()
	previousRedis := common.RedisEnabled
	previousSettings := *system_setting.GetOpenBalanceApiSettings()
	previousDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType

	common.RedisEnabled = false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.OpenCredential{}, &model.Log{},
	))
	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.RedisEnabled = previousRedis
		common.SetMainDatabaseType(previousType)
		*system_setting.GetOpenBalanceApiSettings() = previousSettings
		operation_setting.GetGeneralSetting().QuotaDisplayType = previousDisplayType
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	engine := gin.New()
	SetOpenRouter(engine)
	return engine, db
}

func createOpenBalanceUser(t *testing.T, db *gorm.DB, username string, status int) model.User {
	t.Helper()
	hash, err := common.Password2Hash("correct-horse")
	require.NoError(t, err)
	user := model.User{
		Username:     username,
		Password:     hash,
		DisplayName:  "Alice Example",
		AffCode:      "aff-" + username,
		Status:       status,
		Quota:        500000,
		UsedQuota:    123456,
		RequestCount: 831,
		AuthVersion:  1,
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

// issueBalanceKey mints a key the way the profile endpoint does, so these tests
// exercise the public read path without standing up dashboard authentication.
func issueBalanceKey(t *testing.T, userId int) map[string]string {
	t.Helper()
	key, _, err := model.IssueOpenCredential(userId, "laptop script", "203.0.113.7")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(key, model.OpenCredentialPrefix))
	return map[string]string{"Authorization": "Bearer " + key}
}

func callOpenApi(t *testing.T, engine *gin.Engine, method string, path string, headers map[string]string, body string) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	decoded := map[string]any{}
	if recorder.Body.Len() > 0 {
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &decoded))
	}
	return recorder.Code, decoded
}

func TestOpenBalanceReturnsAccountWideNumbers(t *testing.T) {
	engine, db := setupOpenBalanceApi(t)
	user := createOpenBalanceUser(t, db, "alice", common.UserStatusEnabled)
	authorization := issueBalanceKey(t, user.Id)

	status, body := callOpenApi(t, engine, http.MethodGet, "/api/open/v1/balance", authorization, "")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, true, body["success"])

	// The whole point of this endpoint is the account wallet, not the quota of
	// whichever key happened to be presented.
	balance := body["data"].(map[string]any)
	assert.Equal(t, "alice", balance["username"])
	assert.Equal(t, "Alice Example", balance["display_name"])
	assert.Equal(t, float64(500000), balance["quota"])
	assert.Equal(t, float64(123456), balance["used_quota"])
	assert.Equal(t, float64(831), balance["request_count"])
	// 500000 raw quota units is exactly one display unit at the default rate.
	assert.InDelta(t, 1.0, balance["balance"].(float64), 1e-9)
	assert.InDelta(t, 0.246912, balance["used"].(float64), 1e-9)
	assert.Equal(t, operation_setting.QuotaDisplayTypeUSD, balance["display_type"])
	assert.Equal(t, "$", balance["currency_symbol"])
}

func TestOpenBalanceFollowsSiteQuotaDisplayType(t *testing.T) {
	engine, db := setupOpenBalanceApi(t)
	user := createOpenBalanceUser(t, db, "alice", common.UserStatusEnabled)
	authorization := issueBalanceKey(t, user.Id)

	cases := []struct {
		displayType string
		balance     float64
		symbol      string
	}{
		{operation_setting.QuotaDisplayTypeUSD, 1.0, "$"},
		{operation_setting.QuotaDisplayTypeCNY, 1.0 * operation_setting.USDExchangeRate, "¥"},
		{operation_setting.QuotaDisplayTypeTokens, 500000, ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.displayType, func(t *testing.T) {
			operation_setting.GetGeneralSetting().QuotaDisplayType = testCase.displayType
			status, response := callOpenApi(t, engine, http.MethodGet, "/api/open/v1/balance", authorization, "")
			require.Equal(t, http.StatusOK, status)
			balance := response["data"].(map[string]any)
			assert.InDelta(t, testCase.balance, balance["balance"].(float64), 1e-9)
			assert.Equal(t, testCase.displayType, balance["display_type"])
			assert.Equal(t, testCase.symbol, balance["currency_symbol"])
			// The raw quota is always returned unconverted so a caller can pin a
			// stable number regardless of the site's display setting.
			assert.Equal(t, float64(500000), balance["quota"])
		})
	}
}

func TestOpenBalanceRejectsRevokedAndUnknownCredentials(t *testing.T) {
	engine, db := setupOpenBalanceApi(t)
	user := createOpenBalanceUser(t, db, "alice", common.UserStatusEnabled)
	authorization := issueBalanceKey(t, user.Id)

	status, response := callOpenApi(t, engine, http.MethodGet, "/api/open/v1/balance", nil, "")
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Equal(t, "CREDENTIAL_INVALID", response["code"])

	status, response = callOpenApi(t, engine, http.MethodGet, "/api/open/v1/balance",
		map[string]string{"Authorization": "Bearer obk_nonsense"}, "")
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Equal(t, "CREDENTIAL_INVALID", response["code"])

	status, response = callOpenApi(t, engine, http.MethodPost, "/api/open/v1/auth/revoke", authorization, "")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, true, response["success"])

	status, response = callOpenApi(t, engine, http.MethodGet, "/api/open/v1/balance", authorization, "")
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Equal(t, "CREDENTIAL_REVOKED", response["code"])
}

func TestOpenBalanceRejectsDisabledOwner(t *testing.T) {
	engine, db := setupOpenBalanceApi(t)
	user := createOpenBalanceUser(t, db, "alice", common.UserStatusEnabled)
	authorization := issueBalanceKey(t, user.Id)

	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).
		Update("status", common.UserStatusDisabled).Error)

	status, response := callOpenApi(t, engine, http.MethodGet, "/api/open/v1/balance", authorization, "")
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, "USER_DISABLED", response["code"])
}

func TestOpenRouterExposesSelfServiceSurfaceOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// Both groups live under /api; registering them together proves the radix
	// tree accepts /api/open/v1/* alongside the existing /api/oauth/:provider.
	require.NotPanics(t, func() {
		SetApiRouter(engine)
		SetOpenRouter(engine)
	})

	routes := make(map[string]struct{}, len(engine.Routes()))
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, expected := range []string{
		http.MethodPost + " /api/open/v1/auth/revoke",
		http.MethodGet + " /api/open/v1/balance",
		http.MethodGet + " /api/user/balance-keys",
		http.MethodPost + " /api/user/balance-keys",
		http.MethodDelete + " /api/user/balance-keys/:id",
	} {
		_, ok := routes[expected]
		assert.True(t, ok, "missing route %s", expected)
	}
	// Balance keys are issued by their owner from a signed-in session. Nothing
	// may trade a password for one, and no operator-managed application layer
	// stands between the user and their own balance.
	for _, forbidden := range []string{
		http.MethodPost + " /api/open/v1/auth/exchange",
		http.MethodGet + " /api/open-app/",
		http.MethodPost + " /api/open-app/:id/reset-secret",
	} {
		_, ok := routes[forbidden]
		assert.False(t, ok, "route %s must not exist", forbidden)
	}
}
