package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	securityProofScopeResellerPassword      = "reseller.security.password"
	securityProofScopeResellerPasswordReset = "reseller.security.password_reset"
)

type resellerErrorSpec struct {
	Status  int
	Code    string
	Message string
}

func resellerError(c *gin.Context, err error) {
	spec := resellerErrorSpec{http.StatusInternalServerError, "RESELLER_INTERNAL_ERROR", "站长中心暂时不可用"}
	switch {
	case errors.Is(err, model.ErrResellerNotEnabled):
		spec = resellerErrorSpec{http.StatusForbidden, "RESELLER_NOT_ENABLED", "尚未开通站长中心"}
	case errors.Is(err, model.ErrResellerForbidden), errors.Is(err, model.ErrResellerPricingOwnerInvalid):
		spec = resellerErrorSpec{http.StatusForbidden, "RESELLER_FORBIDDEN", "无权访问该站长资源"}
	case errors.Is(err, model.ErrResellerPricingVersionConflict):
		spec = resellerErrorSpec{http.StatusConflict, "RESELLER_VERSION_CONFLICT", "定价已被更新，请刷新后重试"}
	case errors.Is(err, model.ErrResellerMultiplierOutOfRange), errors.Is(err, model.ErrResellerAmountInvalid):
		spec = resellerErrorSpec{http.StatusUnprocessableEntity, "RESELLER_AMOUNT_INVALID", "金额或倍率超出允许范围"}
	case errors.Is(err, model.ErrResellerQuotaPasswordInvalid):
		spec = resellerErrorSpec{http.StatusForbidden, "RESELLER_QUOTA_PASSWORD_INVALID", "额度密码错误"}
	case errors.Is(err, model.ErrResellerLoginPasswordInvalid):
		spec = resellerErrorSpec{http.StatusForbidden, "RESELLER_LOGIN_PASSWORD_INVALID", "登录密码错误"}
	case errors.Is(err, model.ErrResellerQuotaPasswordConfigured):
		spec = resellerErrorSpec{http.StatusConflict, "RESELLER_QUOTA_PASSWORD_CONFIGURED", "额度密码已经设置"}
	case errors.Is(err, model.ErrResellerQuotaPasswordMissing):
		spec = resellerErrorSpec{http.StatusPreconditionRequired, "RESELLER_QUOTA_PASSWORD_MISSING", "请先设置额度密码"}
	case errors.Is(err, model.ErrResellerOutboundFrozen):
		spec = resellerErrorSpec{http.StatusLocked, "RESELLER_OUTBOUND_FROZEN", "发送额度和签发用户码暂时冻结"}
	case errors.Is(err, model.ErrResellerPreviewInvalid):
		spec = resellerErrorSpec{http.StatusConflict, "RESELLER_PREVIEW_INVALID", "转账预览已失效，请重新预览"}
	case errors.Is(err, model.ErrResellerIdempotencyConflict):
		spec = resellerErrorSpec{http.StatusConflict, "RESELLER_IDEMPOTENCY_CONFLICT", "幂等键已用于其他请求"}
	case errors.Is(err, model.ErrResellerRollingLimit):
		spec = resellerErrorSpec{http.StatusTooManyRequests, "RESELLER_ROLLING_LIMIT", "已超过滚动 24 小时发送限额"}
	case errors.Is(err, model.ErrResellerQuotaInsufficient):
		spec = resellerErrorSpec{http.StatusConflict, "RESELLER_QUOTA_INSUFFICIENT", "可用额度不足"}
	case errors.Is(err, model.ErrResellerVoucherInvalid):
		spec = resellerErrorSpec{http.StatusNotFound, "RESELLER_VOUCHER_INVALID", "用户码无效或已兑换"}
	case errors.Is(err, model.ErrResellerInvitationInvalid), errors.Is(err, model.ErrResellerInvitationExpired):
		spec = resellerErrorSpec{http.StatusConflict, "RESELLER_INVITATION_INVALID", "邀请链接无效或已过期"}
	case errors.Is(err, model.ErrResellerCustomerBound):
		spec = resellerErrorSpec{http.StatusConflict, "RESELLER_CUSTOMER_BOUND", "该用户已归属其他站长，请先解绑"}
	case errors.Is(err, model.ErrResellerSelfBinding):
		spec = resellerErrorSpec{http.StatusUnprocessableEntity, "RESELLER_SELF_BINDING", "站长不能把自己绑定为客户"}
	case errors.Is(err, gorm.ErrRecordNotFound):
		spec = resellerErrorSpec{http.StatusNotFound, "RESELLER_NOT_FOUND", "资源不存在"}
	default:
		common.SysError("reseller api: " + err.Error())
	}
	c.JSON(spec.Status, gin.H{"success": false, "data": gin.H{"code": spec.Code}, "message": spec.Message})
}

func resellerBadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "data": gin.H{"code": "RESELLER_REQUEST_INVALID"}, "message": message})
}

func resellerSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data, "message": ""})
}

func requireActiveReseller(c *gin.Context) (*model.ResellerProfile, bool) {
	profile, err := model.GetResellerProfile(c.GetInt("id"))
	if err != nil {
		resellerError(c, err)
		return nil, false
	}
	if profile.Status != model.ResellerStatusActive {
		resellerError(c, model.ErrResellerForbidden)
		return nil, false
	}
	return profile, true
}

func requireResellerProof(c *gin.Context, scope string) bool {
	return middleware.RequireSecurityProof(c, scope, []string{secureVerificationMethod2FA, secureVerificationMethodPasskey})
}

func authorizeResellerPasswordBootstrap(c *gin.Context, loginPassword string, scope string) bool {
	if strings.TrimSpace(loginPassword) == "" {
		return requireResellerProof(c, scope)
	}
	user, err := model.GetUserById(c.GetInt("id"), true)
	if err != nil {
		resellerError(c, err)
		return false
	}
	if !common.ValidatePasswordAndHash(loginPassword, user.Password) {
		resellerError(c, model.ErrResellerLoginPasswordInvalid)
		return false
	}
	return true
}

func requireResellerIdempotency(c *gin.Context) (string, bool) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" || len(key) > 128 {
		c.JSON(http.StatusPreconditionRequired, gin.H{
			"success": false, "data": gin.H{"code": "RESELLER_IDEMPOTENCY_REQUIRED"}, "message": "需要有效的 Idempotency-Key",
		})
		return "", false
	}
	return key, true
}

func resellerPage(c *gin.Context, items any, total int64) {
	page := common.GetPageQuery(c)
	page.SetItems(items)
	page.SetTotal(int(total))
	resellerSuccess(c, page)
}

func GetResellerStatus(c *gin.Context) {
	summary, err := model.GetResellerStatusSummary(c.GetInt("id"), common.GetTimestamp())
	if err != nil {
		resellerError(c, err)
		return
	}
	resellerSuccess(c, summary)
}

func CreateResellerProfile(c *gin.Context) {
	profile, err := model.CreateResellerProfile(c.GetInt("id"), common.GetTimestamp())
	if err != nil {
		resellerError(c, err)
		return
	}
	recordUserSecurityAudit(c, c.GetInt("id"), "reseller.profile.create", nil)
	resellerSuccess(c, gin.H{"status": profile.Status, "receive_public_id": profile.ReceivePublicId, "pricing_version": profile.PricingVersion})
}

func GetResellerInvitation(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	token, invitation, err := model.GetOrCreateResellerInvitation(c.GetInt("id"), common.GetTimestamp())
	if err != nil {
		resellerError(c, err)
		return
	}
	resellerSuccess(c, gin.H{"path": "/j/" + token, "token": token, "expires_at": invitation.ExpiresAt, "version": invitation.Version})
}

func ListResellerCustomers(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	page := common.GetPageQuery(c)
	items, total, err := model.ListResellerCustomers(c.GetInt("id"), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		resellerError(c, err)
		return
	}
	page.SetItems(items)
	page.SetTotal(int(total))
	resellerSuccess(c, page)
}

func ListResellerTransfers(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	page := common.GetPageQuery(c)
	items, total, err := model.ListResellerTransfers(c.GetInt("id"), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		resellerError(c, err)
		return
	}
	page.SetItems(items)
	page.SetTotal(int(total))
	resellerSuccess(c, page)
}

func ListResellerLedger(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	page := common.GetPageQuery(c)
	items, total, err := model.ListResellerLedger(c.GetInt("id"), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		resellerError(c, err)
		return
	}
	page.SetItems(items)
	page.SetTotal(int(total))
	resellerSuccess(c, page)
}

func GetResellerSecurity(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	security, frozen, err := model.GetResellerSecurityStatus(c.GetInt("id"), common.GetTimestamp())
	if err != nil {
		resellerError(c, err)
		return
	}
	resellerSuccess(c, gin.H{
		"configured": security.PasswordVersion > 0, "password_version": security.PasswordVersion,
		"password_updated_at": security.PasswordUpdatedAt, "outbound_frozen": frozen,
		"outbound_frozen_until": security.OutboundFrozenUntil,
	})
}

func GetDefaultResellerPricing(c *gin.Context) {
	profile, ok := requireActiveReseller(c)
	if !ok {
		return
	}
	rules, err := model.GetResellerPricingRules(model.ResellerPricingOwnerDefault, profile.Id)
	if err != nil {
		resellerError(c, err)
		return
	}
	resellerSuccess(c, gin.H{"pricing_version": profile.PricingVersion, "rules": rules, "multiplier_min_bps": model.ResellerMultiplierBaseBps, "multiplier_max_bps": model.ResellerMultiplierMaxBps})
}

func resellerBindingId(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		resellerBadRequest(c, "客户标识无效")
		return 0, false
	}
	return id, true
}

func GetCustomerResellerPricing(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	bindingId, ok := resellerBindingId(c)
	if !ok {
		return
	}
	binding, err := model.GetResellerOwnedCustomer(c.GetInt("id"), bindingId)
	if err != nil {
		resellerError(c, err)
		return
	}
	rules, err := model.GetResellerPricingRules(model.ResellerPricingOwnerCustomer, binding.Id)
	if err != nil {
		resellerError(c, err)
		return
	}
	resellerSuccess(c, gin.H{"binding_id": binding.Id, "customer_id": binding.CustomerId, "pricing_version": binding.PricingVersion, "rules": rules})
}

type resellerPricingUpdateRequest struct {
	GroupName        string          `json:"group_name"`
	MultiplierBps    int             `json:"multiplier_bps"`
	GroupMultipliers *map[string]int `json:"group_multipliers_bps"`
	ExpectedVersion  int64           `json:"expected_version"`
	QuotaPassword    string          `json:"quota_password"`
	LegacyPassword   string          `json:"password"`
}

func updateResellerPricing(c *gin.Context, ownerType string, ownerId int64, request resellerPricingUpdateRequest) {
	password := request.QuotaPassword
	if password == "" {
		password = request.LegacyPassword
	}
	if err := model.VerifyResellerQuotaPassword(c.GetInt("id"), password, common.GetTimestamp(), false); err != nil {
		resellerError(c, err)
		return
	}
	if request.GroupMultipliers != nil {
		rules, version, err := model.UpdateResellerPricingRules(ownerType, ownerId, request.MultiplierBps, *request.GroupMultipliers, request.ExpectedVersion, common.GetTimestamp())
		if err != nil {
			resellerError(c, err)
			return
		}
		recordUserSecurityAudit(c, c.GetInt("id"), "reseller.pricing.update", map[string]interface{}{
			"owner_type": ownerType, "owner_id": ownerId, "multiplier_bps": request.MultiplierBps, "group_count": len(*request.GroupMultipliers),
		})
		resellerSuccess(c, gin.H{"pricing_version": version, "rules": rules})
		return
	}
	rule, version, err := model.UpdateResellerPricingRule(ownerType, ownerId, request.GroupName, request.MultiplierBps, request.ExpectedVersion, common.GetTimestamp())
	if err != nil {
		resellerError(c, err)
		return
	}
	recordUserSecurityAudit(c, c.GetInt("id"), "reseller.pricing.update", map[string]interface{}{
		"owner_type": ownerType, "owner_id": ownerId, "group_name": strings.TrimSpace(request.GroupName), "multiplier_bps": request.MultiplierBps,
	})
	resellerSuccess(c, gin.H{"pricing_version": version, "rule": rule})
}

func UpdateDefaultResellerPricing(c *gin.Context) {
	profile, ok := requireActiveReseller(c)
	if !ok {
		return
	}
	var request resellerPricingUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resellerBadRequest(c, "定价参数无效")
		return
	}
	updateResellerPricing(c, model.ResellerPricingOwnerDefault, profile.Id, request)
}

func UpdateCustomerResellerPricing(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	bindingId, ok := resellerBindingId(c)
	if !ok {
		return
	}
	binding, err := model.GetResellerOwnedCustomer(c.GetInt("id"), bindingId)
	if err != nil {
		resellerError(c, err)
		return
	}
	var request resellerPricingUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resellerBadRequest(c, "定价参数无效")
		return
	}
	updateResellerPricing(c, model.ResellerPricingOwnerCustomer, binding.Id, request)
}

type resellerPricingDeleteRequest struct {
	GroupName       string `json:"group_name"`
	ExpectedVersion int64  `json:"expected_version"`
	QuotaPassword   string `json:"quota_password"`
	LegacyPassword  string `json:"password"`
}

func deleteResellerPricing(c *gin.Context, ownerType string, ownerId int64, request resellerPricingDeleteRequest) {
	password := request.QuotaPassword
	if password == "" {
		password = request.LegacyPassword
	}
	if err := model.VerifyResellerQuotaPassword(c.GetInt("id"), password, common.GetTimestamp(), false); err != nil {
		resellerError(c, err)
		return
	}
	version, err := model.DeleteResellerPricingRule(ownerType, ownerId, request.GroupName, request.ExpectedVersion)
	if err != nil {
		resellerError(c, err)
		return
	}
	recordUserSecurityAudit(c, c.GetInt("id"), "reseller.pricing.delete", map[string]interface{}{
		"owner_type": ownerType, "owner_id": ownerId, "group_name": strings.TrimSpace(request.GroupName),
	})
	resellerSuccess(c, gin.H{"pricing_version": version})
}

func DeleteDefaultResellerPricing(c *gin.Context) {
	profile, ok := requireActiveReseller(c)
	if !ok {
		return
	}
	var request resellerPricingDeleteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resellerBadRequest(c, "定价参数无效")
		return
	}
	deleteResellerPricing(c, model.ResellerPricingOwnerDefault, profile.Id, request)
}

func DeleteCustomerResellerPricing(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	bindingId, ok := resellerBindingId(c)
	if !ok {
		return
	}
	binding, err := model.GetResellerOwnedCustomer(c.GetInt("id"), bindingId)
	if err != nil {
		resellerError(c, err)
		return
	}
	var request resellerPricingDeleteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resellerBadRequest(c, "定价参数无效")
		return
	}
	deleteResellerPricing(c, model.ResellerPricingOwnerCustomer, binding.Id, request)
}

type resellerPasswordRequest struct {
	Password             string `json:"password"`
	QuotaPassword        string `json:"quota_password"`
	LoginPassword        string `json:"login_password"`
	CurrentPassword      string `json:"current_password"`
	CurrentQuotaPassword string `json:"current_quota_password"`
	NewPassword          string `json:"new_password"`
	NewQuotaPassword     string `json:"new_quota_password"`
}

func SetResellerPassword(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	var request resellerPasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resellerBadRequest(c, "额度密码格式无效")
		return
	}
	if !authorizeResellerPasswordBootstrap(c, request.LoginPassword, securityProofScopeResellerPassword) {
		return
	}
	password := request.QuotaPassword
	if password == "" {
		password = request.Password
	}
	if _, err := model.SetResellerQuotaPassword(c.GetInt("id"), password, common.GetTimestamp()); err != nil {
		resellerError(c, err)
		return
	}
	recordUserSecurityAudit(c, c.GetInt("id"), "reseller.security.password_set", nil)
	resellerSuccess(c, gin.H{"configured": true})
}

func ChangeResellerPassword(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	var request resellerPasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resellerBadRequest(c, "额度密码格式无效")
		return
	}
	currentPassword := request.CurrentQuotaPassword
	if currentPassword == "" {
		currentPassword = request.CurrentPassword
	}
	newPassword := request.NewQuotaPassword
	if newPassword == "" {
		newPassword = request.NewPassword
	}
	if err := model.ChangeResellerQuotaPassword(c.GetInt("id"), currentPassword, newPassword, common.GetTimestamp()); err != nil {
		resellerError(c, err)
		return
	}
	recordUserSecurityAudit(c, c.GetInt("id"), "reseller.security.password_change", nil)
	resellerSuccess(c, gin.H{"changed": true})
}

func ResetResellerPassword(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	var request resellerPasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resellerBadRequest(c, "额度密码格式无效")
		return
	}
	if !authorizeResellerPasswordBootstrap(c, request.LoginPassword, securityProofScopeResellerPasswordReset) {
		return
	}
	newPassword := request.QuotaPassword
	if newPassword == "" {
		newPassword = request.NewPassword
	}
	if err := model.ResetResellerQuotaPassword(c.GetInt("id"), newPassword, common.GetTimestamp()); err != nil {
		resellerError(c, err)
		return
	}
	recordUserSecurityAudit(c, c.GetInt("id"), "reseller.security.password_reset", map[string]interface{}{"outbound_frozen_seconds": model.ResellerOutboundFreezeSeconds})
	resellerSuccess(c, gin.H{"reset": true, "outbound_frozen_until": common.GetTimestamp() + model.ResellerOutboundFreezeSeconds})
}

func RotateResellerReceiveAddress(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	var request resellerRevealRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resellerBadRequest(c, "额度密码格式无效")
		return
	}
	if err := model.VerifyResellerQuotaPassword(c.GetInt("id"), request.quotaPassword(), common.GetTimestamp(), false); err != nil {
		resellerError(c, err)
		return
	}
	receiveId, err := model.RotateResellerReceiveAddress(c.GetInt("id"))
	if err != nil {
		resellerError(c, err)
		return
	}
	recordUserSecurityAudit(c, c.GetInt("id"), "reseller.receive_address.rotate", nil)
	resellerSuccess(c, gin.H{"receive_public_id": receiveId})
}

type resellerTransferPreviewRequest struct {
	RecipientUsername string `json:"recipient_username"`
	RecipientPublicId string `json:"recipient_public_id"`
	ReceivePublicId   string `json:"receive_public_id"`
	Amount            int    `json:"amount"`
}

func PreviewResellerTransfer(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	var request resellerTransferPreviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resellerBadRequest(c, "转账参数无效")
		return
	}
	publicId := request.RecipientPublicId
	if publicId == "" {
		publicId = request.ReceivePublicId
	}
	nonce, preview, err := model.CreateResellerTransferPreviewForRecipient(c.GetInt("id"), request.RecipientUsername, publicId, request.Amount, common.GetTimestamp())
	if err != nil {
		resellerError(c, err)
		return
	}
	name, _ := model.GetUsernameById(preview.ReceiverId, true)
	resellerSuccess(c, gin.H{
		"nonce": nonce, "amount": preview.Amount, "quota": preview.Quota,
		"recipient_user_id": preview.ReceiverId, "recipient_username": name,
		"receiver": gin.H{"user_id": preview.ReceiverId, "username": name}, "expires_at": preview.ExpiresAt,
	})
}

type resellerTransferCommitRequest struct {
	Nonce             string `json:"nonce"`
	Password          string `json:"password"`
	QuotaPassword     string `json:"quota_password"`
	RecipientUserId   int    `json:"recipient_user_id"`
	RecipientUsername string `json:"recipient_username"`
	Amount            int    `json:"amount"`
}

func CommitResellerTransfer(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	key, ok := requireResellerIdempotency(c)
	if !ok {
		return
	}
	var request resellerTransferCommitRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resellerBadRequest(c, "转账参数无效")
		return
	}
	password := request.QuotaPassword
	if password == "" {
		password = request.Password
	}
	transfer, err := model.CommitResellerQuotaTransferForRecipient(c.GetInt("id"), request.Nonce, password, key, model.ResellerTransferCommitExpectation{
		RecipientUserId: request.RecipientUserId, RecipientName: request.RecipientUsername, Amount: request.Amount,
	}, common.GetTimestamp())
	if err != nil {
		resellerError(c, err)
		return
	}
	recordUserSecurityAudit(c, c.GetInt("id"), "reseller.transfer.commit", map[string]interface{}{"result_ref": transfer.PublicId, "receiver_id": transfer.ReceiverId, "amount": transfer.Amount})
	resellerSuccess(c, gin.H{"public_id": transfer.PublicId, "receiver_id": transfer.ReceiverId, "amount": transfer.Amount, "quota": transfer.Quota, "created_at": transfer.CreatedAt})
}

type resellerConvertRequest struct {
	Amount        int                `json:"amount"`
	Quota         resellerQuotaValue `json:"quota"`
	Password      string             `json:"password"`
	QuotaPassword string             `json:"quota_password"`
}

type resellerQuotaValue int64

func (value *resellerQuotaValue) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		*value = 0
		return nil
	}
	raw = strings.Trim(raw, `"`)
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return err
	}
	*value = resellerQuotaValue(parsed)
	return nil
}

func ConvertResellerCommission(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	key, ok := requireResellerIdempotency(c)
	if !ok {
		return
	}
	var request resellerConvertRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resellerBadRequest(c, "转换参数无效")
		return
	}
	password := request.QuotaPassword
	if password == "" {
		password = request.Password
	}
	var ref string
	var convertedQuota int64
	var err error
	if request.Quota > 0 {
		ref, convertedQuota, err = model.ConvertAllResellerCommission(c.GetInt("id"), int64(request.Quota), password, key, common.GetTimestamp())
	} else {
		ref, err = model.ConvertResellerCommission(c.GetInt("id"), request.Amount, password, key, common.GetTimestamp())
		convertedQuota = int64(request.Amount) * int64(common.QuotaPerUnit)
	}
	if err != nil {
		resellerError(c, err)
		return
	}
	recordUserSecurityAudit(c, c.GetInt("id"), "reseller.commission.convert", map[string]interface{}{"result_ref": ref, "quota": convertedQuota})
	resellerSuccess(c, gin.H{"public_id": ref, "quota": convertedQuota, "amount": request.Amount})
}

func ListResellerVouchers(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	page := common.GetPageQuery(c)
	items, total, err := model.ListResellerVouchersByStatus(c.GetInt("id"), strings.TrimSpace(c.Query("status")), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		resellerError(c, err)
		return
	}
	page.SetItems(items)
	page.SetTotal(int(total))
	resellerSuccess(c, page)
}

func ListResellerVoucherBatches(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	page := common.GetPageQuery(c)
	items, total, err := model.ListResellerVoucherBatches(c.GetInt("id"), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		resellerError(c, err)
		return
	}
	page.SetItems(items)
	page.SetTotal(int(total))
	resellerSuccess(c, page)
}

type resellerVoucherIssueRequest struct {
	Count         int    `json:"count"`
	Amount        int    `json:"amount"`
	Note          string `json:"note"`
	Password      string `json:"password"`
	QuotaPassword string `json:"quota_password"`
}

func issueResellerVouchers(c *gin.Context, forceSingle bool) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	key, ok := requireResellerIdempotency(c)
	if !ok {
		return
	}
	var request resellerVoucherIssueRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resellerBadRequest(c, "用户码参数无效")
		return
	}
	if forceSingle {
		request.Count = 1
	}
	password := request.QuotaPassword
	if password == "" {
		password = request.Password
	}
	batch, codes, err := model.IssueResellerVoucherBatch(c.GetInt("id"), request.Count, request.Amount, request.Note, password, key, common.GetTimestamp())
	if err != nil {
		resellerError(c, err)
		return
	}
	recordUserSecurityAudit(c, c.GetInt("id"), "reseller.voucher.issue", map[string]interface{}{"batch_public_id": batch.PublicId, "count": batch.Count, "amount": batch.Amount})
	resellerSuccess(c, gin.H{"batch": batch, "codes": codes})
}

func IssueResellerVoucher(c *gin.Context)      { issueResellerVouchers(c, true) }
func IssueResellerVoucherBatch(c *gin.Context) { issueResellerVouchers(c, false) }

type resellerRevealRequest struct {
	Password      string `json:"password"`
	QuotaPassword string `json:"quota_password"`
}

func (request resellerRevealRequest) quotaPassword() string {
	if request.QuotaPassword != "" {
		return request.QuotaPassword
	}
	return request.Password
}

func RevealResellerVoucher(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	var request resellerRevealRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resellerBadRequest(c, "额度密码格式无效")
		return
	}
	code, err := model.RevealResellerVoucher(c.GetInt("id"), c.Param("id"), request.quotaPassword(), common.GetTimestamp())
	if err != nil {
		resellerError(c, err)
		return
	}
	recordUserSecurityAudit(c, c.GetInt("id"), "reseller.voucher.reveal", map[string]interface{}{"voucher_public_id": c.Param("id")})
	resellerSuccess(c, gin.H{"code": code})
}

func RevealResellerVoucherBatch(c *gin.Context) {
	if _, ok := requireActiveReseller(c); !ok {
		return
	}
	var request resellerRevealRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resellerBadRequest(c, "额度密码格式无效")
		return
	}
	codes, err := model.RevealResellerVoucherBatch(c.GetInt("id"), c.Param("id"), request.quotaPassword(), common.GetTimestamp())
	if err != nil {
		resellerError(c, err)
		return
	}
	recordUserSecurityAudit(c, c.GetInt("id"), "reseller.voucher.batch_reveal", map[string]interface{}{"batch_public_id": c.Param("id")})
	resellerSuccess(c, gin.H{"codes": codes})
}

type resellerVoucherRedeemRequest struct {
	Code string `json:"code"`
}

func RedeemResellerVoucher(c *gin.Context) {
	var request resellerVoucherRedeemRequest
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Code) == "" {
		resellerBadRequest(c, "用户码格式无效")
		return
	}
	quota, err := model.RedeemResellerVoucher(request.Code, c.GetInt("id"), common.GetTimestamp())
	if err != nil {
		resellerError(c, err)
		return
	}
	recordUserSecurityAudit(c, c.GetInt("id"), "reseller.voucher.redeem", map[string]interface{}{"quota": quota})
	resellerSuccess(c, gin.H{"quota": quota})
}

func RetiredAffiliateProgram(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{
		"success": false, "data": gin.H{"code": "AFFILIATE_PROGRAM_RETIRED"},
		"message": "旧邀请返利已停用，请使用站长中心",
	})
}

func RetiredAffiliateTransfer(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{
		"success": false, "data": gin.H{"code": "AFFILIATE_TRANSFER_RETIRED"},
		"message": "旧邀请返利已停用，请使用站长中心收益转换",
	})
}
