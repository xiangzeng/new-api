package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// adminResellerPathUserId reads the :id segment, which addresses a customer on
// the binding routes and a reseller on the roster routes. Both are plain user
// ids; only the role the route assigns them differs.
func adminResellerPathUserId(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		resellerBadRequest(c, "用户标识无效")
		return 0, false
	}
	return id, true
}

// AdminListResellers answers "who runs a reseller center", which no other admin
// screen can tell the operator: the role lives in reseller_profiles, not on the
// user row, so before this endpoint it could only be discovered one user at a
// time through the binding dialog.
func AdminListResellers(c *gin.Context) {
	page := common.GetPageQuery(c)
	items, total, err := model.ListResellerRoster(c.Query("keyword"), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		resellerError(c, err)
		return
	}
	page.SetItems(items)
	page.SetTotal(int(total))
	resellerSuccess(c, page)
}

// AdminListResellerCustomers is the operator's view of one reseller's customers.
// It reuses the reseller-facing query with an explicit reseller id instead of
// the caller's own, so both views stay identical as that projection evolves.
// A target without a profile surfaces as RESELLER_NOT_ENABLED.
func AdminListResellerCustomers(c *gin.Context) {
	resellerId, ok := adminResellerPathUserId(c)
	if !ok {
		return
	}
	if _, err := model.GetResellerProfile(resellerId); err != nil {
		resellerError(c, err)
		return
	}
	page := common.GetPageQuery(c)
	items, total, err := model.ListResellerCustomers(resellerId, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		resellerError(c, err)
		return
	}
	page.SetItems(items)
	page.SetTotal(int(total))
	resellerSuccess(c, page)
}

func GetUserResellerBinding(c *gin.Context) {
	customerId, ok := adminResellerPathUserId(c)
	if !ok {
		return
	}
	binding, err := model.AdminGetResellerBinding(customerId, common.GetTimestamp())
	if err != nil {
		resellerError(c, err)
		return
	}
	resellerSuccess(c, binding)
}

type adminResellerBindRequest struct {
	ResellerId       int    `json:"reseller_id"`
	ResellerUsername string `json:"reseller_username"`
}

func BindUserToReseller(c *gin.Context) {
	customerId, ok := adminResellerPathUserId(c)
	if !ok {
		return
	}
	var request adminResellerBindRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		resellerBadRequest(c, "绑定参数无效")
		return
	}
	binding, err := model.AdminBindResellerCustomer(
		request.ResellerId, request.ResellerUsername, customerId, common.GetTimestamp(),
	)
	if err != nil {
		resellerError(c, err)
		return
	}
	resellerSuccess(c, gin.H{
		"binding_id": binding.Id, "customer_id": binding.CustomerId,
		"reseller_id": binding.ResellerId, "registration_source": binding.RegistrationSource,
		"bound_at": binding.BoundAt,
	})
}

func UnbindUserFromReseller(c *gin.Context) {
	customerId, ok := adminResellerPathUserId(c)
	if !ok {
		return
	}
	if err := model.AdminUnbindResellerCustomer(customerId); err != nil {
		resellerError(c, err)
		return
	}
	resellerSuccess(c, gin.H{"customer_id": customerId, "bound": false})
}
