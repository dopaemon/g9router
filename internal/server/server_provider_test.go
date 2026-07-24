package server

import (
	"testing"

	"g9router/internal/providers"
)

func TestProviderProxyBaseURLUsesTokenPlanRegion(t *testing.T) {
	provider := providers.Provider{ID: "xiaomi-tokenplan", BaseURL: "https://token-plan-sgp.xiaomimimo.com/v1/chat/completions", ProviderSpecificData: map[string]any{"region": "ams"}}
	if got := providerProxyBaseURL(provider); got != "https://token-plan-ams.xiaomimimo.com/v1" {
		t.Fatalf("base URL = %q", got)
	}
}
