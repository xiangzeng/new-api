package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type channelUserUsageResponse struct {
	Success bool                     `json:"success"`
	Message string                   `json:"message"`
	Data    []model.ChannelUserUsage `json:"data"`
}

func setupChannelUserUsageControllerTestDB(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.QuotaData{}))
	require.NoError(t, model.DB.Create(&model.QuotaData{UserID: 1, Username: "alice", ChannelID: 7, ModelName: "gpt-a", UseGroup: "default", CreatedAt: 1100, Count: 2, Quota: 100, TokenUsed: 40}).Error)
	require.NoError(t, model.DB.Create(&model.QuotaData{UserID: 2, Username: "bob", ChannelID: 7, ModelName: "gpt-b", UseGroup: "vip", CreatedAt: 1200, Count: 1, Quota: 170, TokenUsed: 30}).Error)
	require.NoError(t, model.DB.Create(&model.QuotaData{UserID: 3, Username: "carol", ChannelID: 8, ModelName: "gpt-b", UseGroup: "vip", CreatedAt: 1200, Count: 1, Quota: 999, TokenUsed: 30}).Error)
}

func performChannelUserUsage(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "7"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	GetChannelUserUsage(ctx)
	return recorder
}

func TestGetChannelUserUsageRanksUsersOfChannel(t *testing.T) {
	setupChannelUserUsageControllerTestDB(t)

	recorder := performChannelUserUsage(t, "/api/channel/7/user_usage?start_timestamp=1000&end_timestamp=2000")
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload channelUserUsageResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, payload.Message)
	require.Len(t, payload.Data, 2, "其他渠道的用户不得出现")
	assert.Equal(t, "bob", payload.Data[0].Username)
	assert.Equal(t, int64(170), payload.Data[0].Quota)
	assert.Equal(t, "alice", payload.Data[1].Username)
	assert.Equal(t, int64(100), payload.Data[1].Quota)
}

func TestGetChannelUserUsageRejectsBadRange(t *testing.T) {
	setupChannelUserUsageControllerTestDB(t)

	tooLong := performChannelUserUsage(t, "/api/channel/7/user_usage?start_timestamp=1000&end_timestamp=9000000")
	var payload channelUserUsageResponse
	require.NoError(t, common.Unmarshal(tooLong.Body.Bytes(), &payload))
	assert.False(t, payload.Success)
	assert.Contains(t, payload.Message, "90 天")

	reversed := performChannelUserUsage(t, "/api/channel/7/user_usage?start_timestamp=2000&end_timestamp=1000")
	require.NoError(t, common.Unmarshal(reversed.Body.Bytes(), &payload))
	assert.False(t, payload.Success)
}
