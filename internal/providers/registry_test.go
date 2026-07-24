package providers

import "testing"

func TestRegistryDescriptors(t *testing.T) {
	openai, ok := Lookup("openai")
	if !ok || openai.Format != "openai" {
		t.Fatal(openai)
	}
	anthropic, ok := Lookup("anthropic")
	if !ok || anthropic.Format != "claude" || anthropic.Headers["anthropic-version"] == "" {
		t.Fatal(anthropic)
	}
	gemini, ok := Lookup("gemini")
	if !ok || gemini.Format != "gemini" || len(gemini.Models) == 0 {
		t.Fatal(gemini)
	}
}
