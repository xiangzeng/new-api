package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type channelUserUsageResponse struct {
	Success bool                      `json:"success"`
	Message string                    `json:"message"`
	Data    []*model.ChannelUserUsage `json:"data"`
}

func setupChannelUserUsageControllerTestDB(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.QuotaData{}, &model.ResellerCustomer{}))

	groupRatioBackup := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	t.Cleanup(func() { _ = ratio_setting.UpdateGroupRatioByJSONString(groupRatioBackup) })

	// alice 走 vip 分组但配了千人千面 0.5；bob 用默认倍率
	require.NoError(t, model.DB.Create(&model.User{Id: 1, Username: "alice", Group: "default", AffCode: "aff-alice",
		CustomPricing: `{"enabled":true,"groups":{"vip":{"ratio":0.5}}}`}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 2, Username: "bob", Group: "default", AffCode: "aff-bob"}).Error)
	require.NoError(t, model.DB.Create(&model.QuotaData{UserID: 1, Username: "alice", ChannelID: 7, ModelName: "gpt-a", UseGroup: "vip", CreatedAt: 1100, Count: 2, Quota: 100, TokenUsed: 40}).Error)
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

func TestGetChannelUserUsageRanksUsersWithRatios(t *testing.T) {
	setupChannelUserUsageControllerTestDB(t)

	recorder := performChannelUserUsage(t, "/api/channel/7/user_usage?start_timestamp=1000&end_timestamp=2000")
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload channelUserUsageResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success, payload.Message)
	require.Len(t, payload.Data, 2, "其他渠道的用户不得出现")

	bob := payload.Data[0]
	assert.Equal(t, "bob", bob.Username)
	assert.Equal(t, int64(170), bob.Quota)
	require.Len(t, bob.Groups, 1)
	assert.Equal(t, "vip", bob.Groups[0].Group)
	assert.Equal(t, 2.0, bob.Groups[0].Ratio)
	assert.Equal(t, 2.0, bob.Groups[0].DefaultRatio)
	assert.Equal(t, "default", bob.Groups[0].RatioSource)

	alice := payload.Data[1]
	assert.Equal(t, "alice", alice.Username)
	require.Len(t, alice.Groups, 1)
	assert.Equal(t, 0.5, alice.Groups[0].Ratio, "千人千面倍率生效")
	assert.Equal(t, 2.0, alice.Groups[0].DefaultRatio)
	assert.Equal(t, "custom", alice.Groups[0].RatioSource)
	assert.Equal(t, 0.0, alice.Groups[0].ResellerMultiplier, "无站长绑定")
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
