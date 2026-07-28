package server

import (
	"encoding/json"
	"testing"
)

func TestProviderModelBodyStripsAlias(t *testing.T) {
	body := providerModelBody([]byte(`{"model":"cx/gpt-5.5","input":"test"}`), "cx/gpt-5.5", "codex")
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	if request["model"] != "gpt-5.5" {
		t.Fatalf("model = %v", request["model"])
	}
}
