package service

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func cloneRelayInfoForResellerBaseQuote(relayInfo *relaycommon.RelayInfo) *relaycommon.RelayInfo {
	if relayInfo == nil || relayInfo.ResellerPricing == nil || !relayInfo.ResellerPricing.Enabled {
		return nil
	}
	cloned := *relayInfo
	cloned.PriceData = relayInfo.PriceData
	cloned.PriceData.GroupRatioInfo = relayInfo.PriceData.GroupRatioInfo
	cloned.PriceData.GroupRatioInfo.GroupRatio = relayInfo.ResellerPricing.BaseGroupRatio
	cloned.QuotaClamp = nil
	if relayInfo.TieredBillingSnapshot != nil {
		snapshot := *relayInfo.TieredBillingSnapshot
		snapshot.GroupRatio = relayInfo.ResellerPricing.BaseGroupRatio
		baseEstimate, err := billingexpr.QuotaRoundStrict(snapshot.EstimatedQuotaBeforeGroup * snapshot.GroupRatio)
		if err == nil {
			snapshot.EstimatedQuotaAfterGroup = baseEstimate
		}
		cloned.TieredBillingSnapshot = &snapshot
	}
	return &cloned
}

func SetResellerActualQuota(relayInfo *relaycommon.RelayInfo, baseQuota int, retailQuota int) {
	if relayInfo == nil || relayInfo.ResellerPricing == nil {
		return
	}
	relayInfo.ResellerPricing.BaseActualQuota = baseQuota
	relayInfo.ResellerPricing.RetailActualQuota = retailQuota
}

func resellerSettlementReference(relayInfo *relaycommon.RelayInfo) string {
	if relayInfo.ResellerPricing.SettlementReference != "" {
		return relayInfo.ResellerPricing.SettlementReference
	}
	return "request:" + relayInfo.RequestId + ":final"
}

func finalizeResellerCommission(relayInfo *relaycommon.RelayInfo, actualQuota int) error {
	if relayInfo == nil || relayInfo.ResellerPricing == nil ||
		!relayInfo.ResellerPricing.Enabled || relayInfo.ResellerPricing.DeferCommissionUntilTask {
		return nil
	}
	pricing := relayInfo.ResellerPricing
	if pricing.RetailActualQuota == 0 && actualQuota == pricing.RetailPreConsumedQuota {
		pricing.BaseActualQuota = pricing.BasePreConsumedQuota
		pricing.RetailActualQuota = actualQuota
	}
	if pricing.RetailActualQuota != actualQuota {
		return fmt.Errorf("reseller retail quote mismatch: settled=%d quoted=%d", actualQuota, pricing.RetailActualQuota)
	}
	_, err := model.CreateResellerCommission(model.CreateResellerCommissionParams{
		RequestReference:  resellerSettlementReference(relayInfo),
		ResellerId:        pricing.ResellerId,
		CustomerId:        pricing.CustomerId,
		CustomerBindingId: pricing.CustomerBindingId,
		MultiplierBps:     pricing.MultiplierBps,
		MultiplierSource:  pricing.MultiplierSource,
		BaseQuota:         pricing.BaseActualQuota,
		RetailQuota:       pricing.RetailActualQuota,
		Now:               time.Now(),
	})
	return err
}
