package tray

import "testing"

func TestFallbackIconHasPNGSignature(t *testing.T) {
	if len(fallbackIcon) < 8 || string(fallbackIcon[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatal("invalid fallback icon")
	}
}
