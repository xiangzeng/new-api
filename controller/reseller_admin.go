package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func adminResellerCustomerId(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		resellerBadRequest(c, "用户标识无效")
		return 0, false
	}
	return id, true
}

func GetUserResellerBinding(c *gin.Context) {
	customerId, ok := adminResellerCustomerId(c)
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
	customerId, ok := adminResellerCustomerId(c)
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
	customerId, ok := adminResellerCustomerId(c)
	if !ok {
		return
	}
	if err := model.AdminUnbindResellerCustomer(customerId); err != nil {
		resellerError(c, err)
		return
	}
	resellerSuccess(c, gin.H{"customer_id": customerId, "bound": false})
}
