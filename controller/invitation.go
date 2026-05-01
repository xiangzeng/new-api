package controller

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetInvitationSummaries(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	summaries, total, err := model.GetInvitationSummaries(pageInfo, keyword, startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(summaries)
	common.ApiSuccess(c, pageInfo)
}

func GetInvitationInvitees(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	inviterID, _ := strconv.Atoi(c.Query("inviter_id"))
	if inviterID <= 0 {
		common.ApiError(c, errors.New("inviter_id is required"))
		return
	}

	keyword := c.Query("keyword")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	invitees, total, err := model.GetInvitationInvitees(inviterID, pageInfo, keyword, startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(invitees)
	common.ApiSuccess(c, pageInfo)
}
