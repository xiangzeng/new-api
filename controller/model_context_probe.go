package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

var (
	contextLengthCache   = make(map[string]int)
	contextLengthCacheMu sync.RWMutex
)

type probeModelEntry struct {
	Id            string `json:"id"`
	ContextWindow int    `json:"context_window"`
	ContextLength int    `json:"context_length"`
}

type probeModelsResponse struct {
	Data []probeModelEntry `json:"data"`
}

func EnrichModelsWithContextLength(models []dto.OpenAIModels) {
	contextLengthCacheMu.RLock()
	defer contextLengthCacheMu.RUnlock()
	for i := range models {
		if ctxLen, ok := contextLengthCache[models[i].Id]; ok && ctxLen > 0 {
			models[i].ContextLength = ctxLen
		}
	}
}

func StartContextLengthProbe() {
	go func() {
		time.Sleep(15 * time.Second)
		probeAllChannelContextLengths()

		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			probeAllChannelContextLengths()
		}
	}()
}

func probeAllChannelContextLengths() {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		common.SysError("context probe: failed to get channels: " + err.Error())
		return
	}

	newCache := make(map[string]int)

	for _, ch := range channels {
		if ch.Status != common.ChannelStatusEnabled {
			continue
		}
		baseURL := ch.GetBaseURL()
		if baseURL == "" {
			continue
		}

		probeChannelModels(baseURL, ch.Key, newCache)
	}

	if len(newCache) > 0 {
		contextLengthCacheMu.Lock()
		contextLengthCache = newCache
		contextLengthCacheMu.Unlock()
		common.SysLog(fmt.Sprintf("context probe: cached context_length for %d models", len(newCache)))
	}
}

func probeChannelModels(baseURL string, key string, cache map[string]int) {
	url := strings.TrimRight(baseURL, "/") + "/v1/models"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}

	authKey := key
	if idx := strings.Index(authKey, "\n"); idx > 0 {
		authKey = authKey[:idx]
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(authKey))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return
	}

	var modelsResp probeModelsResponse
	if err := json.Unmarshal(body, &modelsResp); err != nil {
		return
	}

	for _, m := range modelsResp.Data {
		ctxLen := m.ContextWindow
		if ctxLen == 0 {
			ctxLen = m.ContextLength
		}
		if ctxLen > 0 {
			if existing, ok := cache[m.Id]; !ok || ctxLen > existing {
				cache[m.Id] = ctxLen
			}
		}
	}
}
