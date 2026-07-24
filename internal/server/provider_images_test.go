package server

import "testing"

func TestModelPathPreservesProviderSlashes(t *testing.T) {
	if got := modelPath("black-forest-labs/FLUX.1-schnell"); got != "black-forest-labs/FLUX.1-schnell" {
		t.Fatalf("model path=%q", got)
	}
}
