package executor

import "testing"

func TestConfigForProviderUsesRegistry(t *testing.T) {
	config, ok := ConfigForProvider("anthropic", "key")
	if !ok || config.Format != "claude" || config.Headers["anthropic-version"] == "" {
		t.Fatal(config, ok)
	}
	if config.APIKey != "key" {
		t.Fatal(config.APIKey)
	}
}
