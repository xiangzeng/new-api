package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type resellerRegistrationOAuthProvider struct{}

func (*resellerRegistrationOAuthProvider) GetName() string { return "Reseller Test" }
func (*resellerRegistrationOAuthProvider) IsEnabled() bool { return true }
func (*resellerRegistrationOAuthProvider) ExchangeToken(context.Context, string, *gin.Context) (*oauth.OAuthToken, error) {
	return &oauth.OAuthToken{}, nil
}
func (*resellerRegistrationOAuthProvider) GetUserInfo(context.Context, *oauth.OAuthToken) (*oauth.OAuthUser, error) {
	return &oauth.OAuthUser{}, nil
}
func (*resellerRegistrationOAuthProvider) IsUserIDTaken(string) bool { return false }
func (*resellerRegistrationOAuthProvider) FillUserByProviderID(*model.User, string) error {
	return nil
}
func (*resellerRegistrationOAuthProvider) SetProviderUserID(*model.User, string) {}
func (*resellerRegistrationOAuthProvider) GetProviderPrefix() string             { return "reseller_oauth_" }

func setupResellerRegistrationControllerTest(t *testing.T) (*gorm.DB, model.User, string) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis := common.RedisEnabled
	previousSecret := common.SessionSecret
	previousRegisterEnabled := common.RegisterEnabled
	previousPasswordRegisterEnabled := common.PasswordRegisterEnabled
	previousEmailVerification := common.EmailVerificationEnabled
	previousQuotaForNewUser := common.QuotaForNewUser
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Log{},
		&model.AuthFlow{},
		&model.ResellerProfile{},
		&model.ResellerCustomer{},
		&model.ResellerInvitation{},
	))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.SessionSecret = "reseller-registration-controller-secret"
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false
	common.QuotaForNewUser = 0

	reseller := model.User{
		Username: "registration-reseller",
		Password: "unused",
		AffCode:  "registration-reseller-aff",
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
		Group:    "default",
	}
	require.NoError(t, db.Create(&reseller).Error)
	profile := model.ResellerProfile{
		UserId:          reseller.Id,
		ReceivePublicId: "12345678901234567890123456789012",
	}
	require.NoError(t, db.Create(&profile).Error)
	token, _, err := model.GetOrCreateResellerInvitation(reseller.Id, common.GetTimestamp())
	require.NoError(t, err)

	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedis
		common.SessionSecret = previousSecret
		common.RegisterEnabled = previousRegisterEnabled
		common.PasswordRegisterEnabled = previousPasswordRegisterEnabled
		common.EmailVerificationEnabled = previousEmailVerification
		common.QuotaForNewUser = previousQuotaForNewUser
		common.SetDatabaseTypes(previousMainType, previousLogType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db, reseller, token
}

func TestPasswordRegistrationAtomicallyBindsDirectReseller(t *testing.T) {
	db, reseller, token := setupResellerRegistrationControllerTest(t)
	gin.SetMode(gin.TestMode)
	body := fmt.Sprintf(`{"username":"password-customer","password":"password123","reseller_invitation":%q}`, token)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	Register(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	var customer model.User
	require.NoError(t, db.Where("username = ?", "password-customer").First(&customer).Error)
	assert.Equal(t, reseller.Id, customer.InviterId)
	var binding model.ResellerCustomer
	require.NoError(t, db.Where("customer_id = ?", customer.Id).First(&binding).Error)
	assert.Equal(t, reseller.Id, binding.ResellerId)
	assert.Equal(t, model.ResellerRegistrationSourceReseller, binding.RegistrationSource)
}

func TestOAuthRegistrationAtomicallyBindsDirectReseller(t *testing.T) {
	db, reseller, token := setupResellerRegistrationControllerTest(t)
	provider := &resellerRegistrationOAuthProvider{}
	invitation, err := model.ResolveResellerInvitation(token, common.GetTimestamp())
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/oauth/reseller-test", nil)

	user, err := findOrCreateOAuthUser(ctx, provider, &oauth.OAuthUser{
		ProviderUserID: "reseller-external-user",
		Username:       "oauth-customer",
		DisplayName:    "OAuth Customer",
	}, invitation.Id, invitation.Version)
	require.NoError(t, err)
	assert.Equal(t, reseller.Id, user.InviterId)
	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, reseller.Id, stored.InviterId)
	var binding model.ResellerCustomer
	require.NoError(t, db.Where("customer_id = ?", user.Id).First(&binding).Error)
	assert.Equal(t, reseller.Id, binding.ResellerId)
}

func TestPasswordRegistrationRejectsRetiredAffiliateProgram(t *testing.T) {
	db, reseller, _ := setupResellerRegistrationControllerTest(t)
	body := fmt.Sprintf(`{"username":"legacy-aff-customer","password":"password123","aff_code":%q}`, reseller.AffCode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	Register(ctx)

	assert.Equal(t, http.StatusGone, recorder.Code)
	assert.Equal(t, "AFFILIATE_PROGRAM_RETIRED", decodedResellerResponse(t, recorder)["data"].(map[string]any)["code"])
	var count int64
	require.NoError(t, db.Model(&model.User{}).Where("username = ?", "legacy-aff-customer").Count(&count).Error)
	assert.Zero(t, count)
}

func TestOAuthStateRejectsRetiredAffiliateProgram(t *testing.T) {
	setupResellerRegistrationControllerTest(t)
	const providerName = "reseller-retired-aff"
	oauth.Register(providerName, &resellerRegistrationOAuthProvider{})
	t.Cleanup(func() { oauth.Unregister(providerName) })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/oauth/state", strings.NewReader(
		`{"provider":"reseller-retired-aff","intent":"login","aff":"legacy"}`,
	))
	ctx.Request.Header.Set("Content-Type", "application/json")

	GenerateOAuthCode(ctx)

	assert.Equal(t, http.StatusGone, recorder.Code)
	assert.Equal(t, "AFFILIATE_PROGRAM_RETIRED", decodedResellerResponse(t, recorder)["data"].(map[string]any)["code"])
}

func TestLegacyFinalizerDoesNotGrantFixedAffiliateRewards(t *testing.T) {
	db, reseller, _ := setupResellerRegistrationControllerTest(t)
	previousInviterQuota, previousInviteeQuota := common.QuotaForInviter, common.QuotaForInvitee
	common.QuotaForInviter, common.QuotaForInvitee = 1000, 500
	t.Cleanup(func() {
		common.QuotaForInviter, common.QuotaForInvitee = previousInviterQuota, previousInviteeQuota
	})
	customer := model.User{
		Username: "legacy-finalizer-customer",
		AffCode:  "legacy-finalizer-customer-aff",
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
		Group:    "default",
	}
	require.NoError(t, db.Create(&customer).Error)

	customer.FinishInsert(reseller.Id)

	var persistedCustomer, persistedReseller model.User
	require.NoError(t, db.First(&persistedCustomer, customer.Id).Error)
	require.NoError(t, db.First(&persistedReseller, reseller.Id).Error)
	assert.Zero(t, persistedCustomer.Quota)
	assert.Zero(t, persistedReseller.AffCount)
	assert.Zero(t, persistedReseller.AffQuota)
	assert.Zero(t, persistedReseller.AffHistoryQuota)
}
