package service

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resellerQuoteTestContext() *gin.Context {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	return ctx
}

func TestTextResellerQuoteRunsSameRatioBillingTwice(t *testing.T) {
	info := &relaycommon.RelayInfo{
		OriginModelName: "reseller-ratio-model",
		PriceData: hosttypes.PriceData{
			ModelRatio:      1,
			CompletionRatio: 2,
			CacheRatio:      0.5,
			GroupRatioInfo:  hosttypes.GroupRatioInfo{GroupRatio: 1.25, BaseGroupRatio: 1},
		},
		ResellerPricing: &hosttypes.ResellerPricingSnapshot{
			Enabled: true, BaseGroupRatio: 1, RetailGroupRatio: 1.25,
		},
	}
	usage := &dto.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}

	retail := calculateTextQuotaSummary(resellerQuoteTestContext(), info, usage)
	baseInfo := cloneRelayInfoForResellerBaseQuote(info)
	require.NotNil(t, baseInfo)
	base := calculateTextQuotaSummary(resellerQuoteTestContext(), baseInfo, usage)

	assert.Equal(t, 7, base.Quota)
	assert.Equal(t, 9, retail.Quota)
	assert.Equal(t, 1.0, base.GroupRatio)
	assert.Equal(t, 1.25, retail.GroupRatio)
}

func TestFixedPriceAndAudioResellerQuotesPreserveRounding(t *testing.T) {
	fixedBase, _ := calculateAudioQuota(QuotaInfo{UsePrice: true, ModelPrice: 0.00011, GroupRatio: 1})
	fixedRetail, _ := calculateAudioQuota(QuotaInfo{UsePrice: true, ModelPrice: 0.00011, GroupRatio: 1.5})
	assert.Equal(t, 55, fixedBase)
	assert.Equal(t, 83, fixedRetail)

	base, _ := calculateAudioQuota(QuotaInfo{
		InputDetails:  TokenDetails{TextTokens: 3, AudioTokens: 2},
		OutputDetails: TokenDetails{TextTokens: 1, AudioTokens: 1},
		ModelName:     "gpt-4o-realtime-preview", ModelRatio: 1, GroupRatio: 1,
	})
	retail, _ := calculateAudioQuota(QuotaInfo{
		InputDetails:  TokenDetails{TextTokens: 3, AudioTokens: 2},
		OutputDetails: TokenDetails{TextTokens: 1, AudioTokens: 1},
		ModelName:     "gpt-4o-realtime-preview", ModelRatio: 1, GroupRatio: 1.5,
	})
	assert.GreaterOrEqual(t, retail, base)
	assert.NotEqual(t, base, retail)
}
