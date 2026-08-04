package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupResellerControllerTest(t *testing.T) (*gorm.DB, model.User, model.ResellerProfile) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis, previousSecret := common.RedisEnabled, common.SessionSecret
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Log{}, &model.ResellerProfile{}, &model.ResellerCustomer{},
		&model.ResellerPricingRule{}, &model.ResellerInvitation{}, &model.ResellerCommissionEntry{},
		&model.ResellerLedgerTransaction{}, &model.ResellerLedgerLine{}, &model.ResellerSecurity{},
		&model.ResellerTransferPreview{}, &model.ResellerIdempotencyRecord{}, &model.ResellerOutboundEvent{},
		&model.ResellerQuotaTransfer{}, &model.ResellerVoucherBatch{}, &model.ResellerVoucher{},
	))
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled = false
	common.SessionSecret = "reseller-controller-test-secret"
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	loginPasswordHash, err := common.Password2Hash("login-password")
	require.NoError(t, err)
	user := model.User{Username: "reseller-api-owner", Password: loginPasswordHash, AffCode: "reseller-owner-aff", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", Quota: 1000000, AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	profile := model.ResellerProfile{UserId: user.Id, Status: model.ResellerStatusActive, ReceivePublicId: "rr_owner_public", PricingVersion: 1}
	require.NoError(t, db.Create(&profile).Error)
	_, err = model.SetResellerQuotaPassword(user.Id, "123456", 100)
	require.NoError(t, err)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled, common.SessionSecret = previousRedis, previousSecret
		common.SetDatabaseTypes(previousMainType, previousLogType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db, user, profile
}

func resellerTestContext(method string, path string, body string, user model.User) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", user.Id)
	ctx.Set("username", user.Username)
	ctx.Set("auth_identity", service.AuthIdentity{UserID: user.Id, SessionID: "reseller-session", UserAuthVersion: 1, SessionVersion: 1})
	return ctx, recorder
}

func resellerProof(t *testing.T, identity service.AuthIdentity, scope string) string {
	t.Helper()
	proof, _, err := service.IssueSecurityProof(identity, secureVerificationMethod2FA, []string{scope})
	require.NoError(t, err)
	return proof
}

func decodedResellerResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var response map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestResellerCustomerPricingIsOwnerScoped(t *testing.T) {
	db, owner, _ := setupResellerControllerTest(t)
	other := model.User{Username: "other-reseller", AffCode: "other-reseller-aff", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default"}
	customer := model.User{Username: "other-customer", AffCode: "other-customer-aff", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default"}
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&customer).Error)
	require.NoError(t, db.Create(&model.ResellerProfile{UserId: other.Id, Status: model.ResellerStatusActive, ReceivePublicId: "rr_other_public"}).Error)
	binding := model.ResellerCustomer{ResellerId: other.Id, CustomerId: customer.Id, RegistrationSource: model.ResellerRegistrationSourceReseller, BoundAt: common.GetTimestamp(), PricingVersion: 1}
	require.NoError(t, db.Create(&binding).Error)

	ctx, recorder := resellerTestContext(http.MethodGet, fmt.Sprintf("/api/reseller/customers/%d/pricing", binding.Id), "", owner)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(binding.Id)}}
	GetCustomerResellerPricing(ctx)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Equal(t, "RESELLER_FORBIDDEN", decodedResellerResponse(t, recorder)["data"].(map[string]any)["code"])
}

func TestResellerStatusExposesOnlyObservedOverviewStats(t *testing.T) {
	db, owner, profile := setupResellerControllerTest(t)
	require.NoError(t, db.Model(&model.ResellerProfile{}).Where("id = ?", profile.Id).Updates(map[string]any{
		"pending_commission_quota":   120,
		"available_commission_quota": 80,
	}).Error)
	require.NoError(t, db.Create(&model.ResellerOutboundEvent{
		UserId: owner.Id, Kind: "transfer", Amount: 500, Reference: "status-hidden-outbound-total", CreatedAt: common.GetTimestamp(),
	}).Error)

	ctx, recorder := resellerTestContext(http.MethodGet, "/api/reseller/status", "", owner)
	GetResellerStatus(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	data := decodedResellerResponse(t, recorder)["data"].(map[string]any)
	assert.EqualValues(t, owner.Quota, data["wallet_quota"])
	assert.EqualValues(t, 120, data["pending_commission_quota"])
	assert.EqualValues(t, 80, data["available_commission_quota"])
	assert.EqualValues(t, 0, data["customer_count"])
	assert.NotContains(t, data, "outbound_used_24h")
}

func TestResellerPricingConflictUsesStable409Envelope(t *testing.T) {
	_, owner, profile := setupResellerControllerTest(t)
	body := `{"group_name":"default","multiplier_bps":12000,"expected_version":99,"quota_password":"123456"}`
	ctx, recorder := resellerTestContext(http.MethodPut, "/api/reseller/pricing/default", body, owner)
	UpdateDefaultResellerPricing(ctx)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	response := decodedResellerResponse(t, recorder)
	assert.Equal(t, "RESELLER_VERSION_CONFLICT", response["data"].(map[string]any)["code"])
	var persisted model.ResellerProfile
	require.NoError(t, model.DB.First(&persisted, profile.Id).Error)
	assert.Equal(t, int64(1), persisted.PricingVersion)
}

func TestResellerPricingDocumentUpdatesOverallAndGroupsOnce(t *testing.T) {
	_, owner, profile := setupResellerControllerTest(t)
	body := `{"multiplier_bps":12500,"group_multipliers_bps":{"pro":14000,"vip":15000},"expected_version":1,"quota_password":"123456"}`
	ctx, recorder := resellerTestContext(http.MethodPut, "/api/reseller/pricing/default", body, owner)
	UpdateDefaultResellerPricing(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodedResellerResponse(t, recorder)["data"].(map[string]any)
	assert.EqualValues(t, 2, response["pricing_version"])
	rules, err := model.GetResellerPricingRules(model.ResellerPricingOwnerDefault, profile.Id)
	require.NoError(t, err)
	assert.Len(t, rules, 3)
	assert.Equal(t, 12500, rules[""].CurrentMultiplierBps)
	assert.Equal(t, 14000, rules["pro"].CurrentMultiplierBps)
	assert.Equal(t, 15000, rules["vip"].CurrentMultiplierBps)
}

func TestDeleteDefaultResellerPricingRejectsStaleVersion(t *testing.T) {
	_, owner, profile := setupResellerControllerTest(t)
	_, version, err := model.UpdateResellerPricingRule(model.ResellerPricingOwnerDefault, profile.Id, "pro", 15000, 1, common.GetTimestamp())
	require.NoError(t, err)
	assert.Equal(t, int64(2), version)

	ctx, recorder := resellerTestContext(http.MethodDelete, "/api/reseller/pricing/default", `{"group_name":"pro","expected_version":1,"quota_password":"123456"}`, owner)
	DeleteDefaultResellerPricing(ctx)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Equal(t, "RESELLER_VERSION_CONFLICT", decodedResellerResponse(t, recorder)["data"].(map[string]any)["code"])
	rules, err := model.GetResellerPricingRules(model.ResellerPricingOwnerDefault, profile.Id)
	require.NoError(t, err)
	assert.Contains(t, rules, "pro")
}

func TestDeleteCustomerResellerPricingIsOwnerScoped(t *testing.T) {
	db, owner, _ := setupResellerControllerTest(t)
	other := model.User{Username: "delete-other-reseller", AffCode: "delete-other-reseller-aff", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default"}
	customer := model.User{Username: "delete-other-customer", AffCode: "delete-other-customer-aff", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default"}
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&customer).Error)
	binding := model.ResellerCustomer{ResellerId: other.Id, CustomerId: customer.Id, RegistrationSource: model.ResellerRegistrationSourceReseller, BoundAt: common.GetTimestamp(), PricingVersion: 1}
	require.NoError(t, db.Create(&binding).Error)
	_, version, err := model.UpdateResellerPricingRule(model.ResellerPricingOwnerCustomer, binding.Id, "pro", 15000, 1, common.GetTimestamp())
	require.NoError(t, err)

	ctx, recorder := resellerTestContext(http.MethodDelete, fmt.Sprintf("/api/reseller/customers/%d/pricing", binding.Id), fmt.Sprintf(`{"group_name":"pro","expected_version":%d}`, version), owner)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(binding.Id)}}
	DeleteCustomerResellerPricing(ctx)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Equal(t, "RESELLER_FORBIDDEN", decodedResellerResponse(t, recorder)["data"].(map[string]any)["code"])
	rules, err := model.GetResellerPricingRules(model.ResellerPricingOwnerCustomer, binding.Id)
	require.NoError(t, err)
	assert.Contains(t, rules, "pro")
}

func TestResellerDailyWritesUseQuotaPasswordWithoutSecurityProof(t *testing.T) {
	db, owner, _ := setupResellerControllerTest(t)
	receiver := model.User{Username: "preview-recipient", AffCode: "preview-recipient-aff", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default"}
	require.NoError(t, db.Create(&receiver).Error)
	ctx, recorder := resellerTestContext(http.MethodPost, "/api/reseller/transfers/preview", `{"recipient_username":"preview-recipient","amount":1}`, owner)
	PreviewResellerTransfer(ctx)
	assert.Equal(t, http.StatusOK, recorder.Code)
	preview := decodedResellerResponse(t, recorder)["data"].(map[string]any)
	assert.Equal(t, "preview-recipient", preview["recipient_username"])

	ctx, recorder = resellerTestContext(http.MethodPost, "/api/reseller/commission/convert", `{"quota":"500000","quota_password":"123456"}`, owner)
	ConvertResellerCommission(ctx)
	assert.Equal(t, http.StatusPreconditionRequired, recorder.Code)
	assert.Equal(t, "RESELLER_IDEMPOTENCY_REQUIRED", decodedResellerResponse(t, recorder)["data"].(map[string]any)["code"])
}

func TestResellerPasswordSetupAcceptsLoginPasswordAndRejectsWrongPassword(t *testing.T) {
	db, owner, _ := setupResellerControllerTest(t)
	require.NoError(t, db.Where("user_id = ?", owner.Id).Delete(&model.ResellerSecurity{}).Error)

	ctx, recorder := resellerTestContext(http.MethodPost, "/api/reseller/security/password", `{"quota_password":"654321","login_password":"wrong"}`, owner)
	SetResellerPassword(ctx)
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Equal(t, "RESELLER_LOGIN_PASSWORD_INVALID", decodedResellerResponse(t, recorder)["data"].(map[string]any)["code"])

	ctx, recorder = resellerTestContext(http.MethodPost, "/api/reseller/security/password", `{"quota_password":"654321","login_password":"login-password"}`, owner)
	SetResellerPassword(ctx)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.NoError(t, model.VerifyResellerQuotaPassword(owner.Id, "654321", 101, false))
}

func TestResellerReadResponsesDoNotLeakStoredSecrets(t *testing.T) {
	db, owner, _ := setupResellerControllerTest(t)
	require.NoError(t, db.Model(&model.ResellerSecurity{}).Where("user_id = ?", owner.Id).Updates(map[string]any{"quota_password_hash": "should-never-leak", "password_version": 3}).Error)
	batch := model.ResellerVoucherBatch{PublicId: "rvb_safe", IssuerId: owner.Id, Count: 1, Amount: 2, TotalQuota: 1000}
	require.NoError(t, db.Create(&batch).Error)
	require.NoError(t, db.Create(&model.ResellerVoucher{PublicId: "rvc_safe", BatchId: batch.Id, IssuerId: owner.Id, CodeDigest: "digest-secret", CodeCiphertext: "cipher-secret", Amount: 2, Quota: 1000}).Error)

	ctx, securityRecorder := resellerTestContext(http.MethodGet, "/api/reseller/security", "", owner)
	GetResellerSecurity(ctx)
	ctx, voucherRecorder := resellerTestContext(http.MethodGet, "/api/reseller/vouchers", "", owner)
	ListResellerVouchers(ctx)

	combined := securityRecorder.Body.String() + voucherRecorder.Body.String()
	assert.NotContains(t, combined, "should-never-leak")
	assert.NotContains(t, combined, "digest-secret")
	assert.NotContains(t, combined, "cipher-secret")
	assert.NotContains(t, combined, "quota_password_hash")
	assert.NotContains(t, combined, "code_ciphertext")
}

func TestEmptyResellerCollectionsUseArrayEnvelope(t *testing.T) {
	_, owner, _ := setupResellerControllerTest(t)
	tests := []struct {
		name    string
		path    string
		handler func(*gin.Context)
	}{
		{name: "ledger", path: "/api/reseller/ledger", handler: ListResellerLedger},
		{name: "transfers", path: "/api/reseller/transfers", handler: ListResellerTransfers},
		{name: "voucher batches", path: "/api/reseller/vouchers/batches", handler: ListResellerVoucherBatches},
		{name: "vouchers", path: "/api/reseller/vouchers", handler: ListResellerVouchers},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, recorder := resellerTestContext(http.MethodGet, test.path, "", owner)
			test.handler(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			response := decodedResellerResponse(t, recorder)
			page := response["data"].(map[string]any)
			assert.Equal(t, []any{}, page["items"])
		})
	}
}

func TestRetiredAffiliateTransferReturnsGone(t *testing.T) {
	_, owner, _ := setupResellerControllerTest(t)
	ctx, recorder := resellerTestContext(http.MethodPost, "/api/user/aff_transfer", `{}`, owner)
	RetiredAffiliateTransfer(ctx)
	assert.Equal(t, http.StatusGone, recorder.Code)
	assert.Equal(t, "AFFILIATE_TRANSFER_RETIRED", decodedResellerResponse(t, recorder)["data"].(map[string]any)["code"])
}

func TestRetiredAffiliateProgramReturnsGone(t *testing.T) {
	_, owner, _ := setupResellerControllerTest(t)
	ctx, recorder := resellerTestContext(http.MethodGet, "/api/user/aff", "", owner)
	RetiredAffiliateProgram(ctx)
	assert.Equal(t, http.StatusGone, recorder.Code)
	assert.Equal(t, "AFFILIATE_PROGRAM_RETIRED", decodedResellerResponse(t, recorder)["data"].(map[string]any)["code"])
}
