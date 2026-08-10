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
	previousSecret := common.SessionSecret
	previousType := common.MainDatabaseType()
	previousRedis := common.RedisEnabled
	previousSettings := *system_setting.GetOpenBalanceApiSettings()
	previousDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType

	common.SessionSecret = "open-router-test-secret"
	common.RedisEnabled = false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.TwoFA{}, &model.OpenApp{}, &model.OpenCredential{}, &model.Log{},
	))
	model.DB = db
	model.LOG_DB = db

	settings := system_setting.GetOpenBalanceApiSettings()
	settings.Enabled = true

	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.SessionSecret = previousSecret
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

func createOpenBalanceUser(t *testing.T, db *gorm.DB, username string, password string, status int) model.User {
	t.Helper()
	hash, err := common.Password2Hash(password)
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

func exchangeHeaders(appId string, secret string) map[string]string {
	return map[string]string{"X-App-Id": appId, "X-App-Secret": secret}
}

func loginBody(username string, password string) string {
	return fmt.Sprintf(`{"username":%q,"password":%q,"end_user_ip":"198.51.100.4"}`, username, password)
}

func TestOpenExchangeIssuesCredentialAndBalanceReads(t *testing.T) {
	engine, db := setupOpenBalanceApi(t)
	app, secret, err := model.CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	createOpenBalanceUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)

	status, body := callOpenApi(t, engine, http.MethodPost, "/api/open/v1/auth/exchange",
		exchangeHeaders(app.AppId, secret), loginBody("alice", "correct-horse"))
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, true, body["success"])

	data := body["data"].(map[string]any)
	credential := data["credential"].(string)
	assert.True(t, strings.HasPrefix(credential, model.OpenCredentialPrefix))
	assert.Equal(t, model.OpenScopeBalanceRead, data["scope"])
	assert.Equal(t, "alice", data["user"].(map[string]any)["username"])

	status, body = callOpenApi(t, engine, http.MethodGet, "/api/open/v1/balance",
		map[string]string{"Authorization": "Bearer " + credential}, "")
	require.Equal(t, http.StatusOK, status)
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
	app, secret, err := model.CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	createOpenBalanceUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)

	_, body := callOpenApi(t, engine, http.MethodPost, "/api/open/v1/auth/exchange",
		exchangeHeaders(app.AppId, secret), loginBody("alice", "correct-horse"))
	credential := body["data"].(map[string]any)["credential"].(string)
	authorization := map[string]string{"Authorization": "Bearer " + credential}

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
			// The raw quota is always returned unconverted so a partner can
			// pin a stable number regardless of the site's display setting.
			assert.Equal(t, float64(500000), balance["quota"])
		})
	}
}

func TestOpenExchangeRejectionContract(t *testing.T) {
	engine, db := setupOpenBalanceApi(t)
	app, secret, err := model.CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	restricted, restrictedSecret, err := model.CreateOpenApp("Restricted Site", "203.0.113.0/24", 0)
	require.NoError(t, err)
	disabled, disabledSecret, err := model.CreateOpenApp("Disabled Site", "", 0)
	require.NoError(t, err)
	_, err = model.UpdateOpenApp(disabled.Id, "Disabled Site", "", model.OpenAppStatusDisabled, 0)
	require.NoError(t, err)

	createOpenBalanceUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	createOpenBalanceUser(t, db, "carol", "tr0ub4dor", common.UserStatusDisabled)

	cases := []struct {
		name    string
		headers map[string]string
		body    string
		status  int
		code    string
	}{
		{"missing app headers", nil, loginBody("alice", "correct-horse"),
			http.StatusUnauthorized, "APP_UNAUTHORIZED"},
		{"wrong app secret", exchangeHeaders(app.AppId, secret+"x"), loginBody("alice", "correct-horse"),
			http.StatusUnauthorized, "APP_UNAUTHORIZED"},
		{"unknown app id", exchangeHeaders("oapp_nope", secret), loginBody("alice", "correct-horse"),
			http.StatusUnauthorized, "APP_UNAUTHORIZED"},
		{"disabled app", exchangeHeaders(disabled.AppId, disabledSecret), loginBody("alice", "correct-horse"),
			http.StatusForbidden, "APP_DISABLED"},
		{"source ip not allowed", exchangeHeaders(restricted.AppId, restrictedSecret), loginBody("alice", "correct-horse"),
			http.StatusForbidden, "APP_IP_NOT_ALLOWED"},
		{"malformed body", exchangeHeaders(app.AppId, secret), `{"username":`,
			http.StatusBadRequest, "INVALID_PARAMS"},
		{"missing password", exchangeHeaders(app.AppId, secret), `{"username":"alice"}`,
			http.StatusBadRequest, "INVALID_PARAMS"},
		{"wrong password", exchangeHeaders(app.AppId, secret), loginBody("alice", "nope"),
			http.StatusUnauthorized, "INVALID_CREDENTIALS"},
		{"unknown user", exchangeHeaders(app.AppId, secret), loginBody("nobody", "correct-horse"),
			http.StatusUnauthorized, "INVALID_CREDENTIALS"},
		{"disabled user", exchangeHeaders(app.AppId, secret), loginBody("carol", "tr0ub4dor"),
			http.StatusForbidden, "USER_DISABLED"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			status, body := callOpenApi(t, engine, http.MethodPost, "/api/open/v1/auth/exchange",
				testCase.headers, testCase.body)
			assert.Equal(t, testCase.status, status)
			assert.Equal(t, false, body["success"])
			assert.Equal(t, testCase.code, body["code"])
		})
	}
}

func TestOpenExchangeRejectsTwoFactorAccounts(t *testing.T) {
	engine, db := setupOpenBalanceApi(t)
	app, secret, err := model.CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	user := createOpenBalanceUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	require.NoError(t, db.Create(&model.TwoFA{UserId: user.Id, Secret: "TOTPSECRET", IsEnabled: true}).Error)

	// A password alone is not the factor set this account chose; accepting it
	// would silently downgrade the user's own security decision.
	status, body := callOpenApi(t, engine, http.MethodPost, "/api/open/v1/auth/exchange",
		exchangeHeaders(app.AppId, secret), loginBody("alice", "correct-horse"))
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, "REQUIRE_2FA_UNSUPPORTED", body["code"])
}

func TestOpenExchangeLocksOutRepeatedPasswordFailures(t *testing.T) {
	engine, db := setupOpenBalanceApi(t)
	settings := system_setting.GetOpenBalanceApiSettings()
	settings.FailureLockThreshold = 2
	settings.FailureLockMinutes = 15

	app, secret, err := model.CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	createOpenBalanceUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	headers := exchangeHeaders(app.AppId, secret)

	for range settings.FailureLockThreshold {
		status, body := callOpenApi(t, engine, http.MethodPost, "/api/open/v1/auth/exchange",
			headers, loginBody("alice", "nope"))
		require.Equal(t, http.StatusUnauthorized, status)
		require.Equal(t, "INVALID_CREDENTIALS", body["code"])
	}

	// Once locked, even the correct password is refused for the window.
	status, body := callOpenApi(t, engine, http.MethodPost, "/api/open/v1/auth/exchange",
		headers, loginBody("alice", "correct-horse"))
	assert.Equal(t, http.StatusTooManyRequests, status)
	assert.Equal(t, "RATE_LIMITED", body["code"])
}

func TestOpenExchangeClearsLockoutAfterSuccess(t *testing.T) {
	engine, db := setupOpenBalanceApi(t)
	settings := system_setting.GetOpenBalanceApiSettings()
	settings.FailureLockThreshold = 3

	app, secret, err := model.CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	createOpenBalanceUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	headers := exchangeHeaders(app.AppId, secret)

	status, _ := callOpenApi(t, engine, http.MethodPost, "/api/open/v1/auth/exchange",
		headers, loginBody("alice", "nope"))
	require.Equal(t, http.StatusUnauthorized, status)

	status, _ = callOpenApi(t, engine, http.MethodPost, "/api/open/v1/auth/exchange",
		headers, loginBody("alice", "correct-horse"))
	require.Equal(t, http.StatusOK, status)

	// A user who mistypes and then gets it right must start from a clean slate.
	for range settings.FailureLockThreshold - 1 {
		status, body := callOpenApi(t, engine, http.MethodPost, "/api/open/v1/auth/exchange",
			headers, loginBody("alice", "nope"))
		require.Equal(t, http.StatusUnauthorized, status)
		require.Equal(t, "INVALID_CREDENTIALS", body["code"])
	}
}

func TestOpenBalanceRejectsRevokedAndUnknownCredentials(t *testing.T) {
	engine, db := setupOpenBalanceApi(t)
	app, secret, err := model.CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	createOpenBalanceUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)

	_, body := callOpenApi(t, engine, http.MethodPost, "/api/open/v1/auth/exchange",
		exchangeHeaders(app.AppId, secret), loginBody("alice", "correct-horse"))
	credential := body["data"].(map[string]any)["credential"].(string)
	authorization := map[string]string{"Authorization": "Bearer " + credential}

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

func TestOpenApiStaysDarkWhenDisabled(t *testing.T) {
	engine, db := setupOpenBalanceApi(t)
	app, secret, err := model.CreateOpenApp("Partner Site", "", 0)
	require.NoError(t, err)
	createOpenBalanceUser(t, db, "alice", "correct-horse", common.UserStatusEnabled)
	system_setting.GetOpenBalanceApiSettings().Enabled = false

	status, body := callOpenApi(t, engine, http.MethodPost, "/api/open/v1/auth/exchange",
		exchangeHeaders(app.AppId, secret), loginBody("alice", "correct-horse"))
	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Equal(t, "OPEN_API_DISABLED", body["code"])

	status, body = callOpenApi(t, engine, http.MethodGet, "/api/open/v1/balance",
		map[string]string{"Authorization": "Bearer obk_whatever"}, "")
	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Equal(t, "OPEN_API_DISABLED", body["code"])
}

func TestOpenRouterDoesNotConflictWithApiRoutes(t *testing.T) {
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
		http.MethodPost + " /api/open/v1/auth/exchange",
		http.MethodPost + " /api/open/v1/auth/revoke",
		http.MethodGet + " /api/open/v1/balance",
		http.MethodGet + " /api/user/open-credentials",
		http.MethodDelete + " /api/user/open-credentials/:id",
		http.MethodPost + " /api/open-app/:id/reset-secret",
	} {
		_, ok := routes[expected]
		assert.True(t, ok, "missing route %s", expected)
	}
}
