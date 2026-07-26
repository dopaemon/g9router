package xai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgsDefaultsAndOptions(t *testing.T) {
	opts, help, err := parseArgs([]string{"--prompt", "cat", "--output", "clip.mp4", "--port", "2020", "--timeout", "12"})
	if err != nil || help != "" {
		t.Fatalf("parseArgs() = %#v, %q, %v", opts, help, err)
	}
	if opts.Prompt != "cat" || opts.Output != "clip.mp4" || opts.Port != 2020 || opts.Timeout.Seconds() != 12 {
		t.Fatalf("unexpected options: %#v", opts)
	}
}

func TestImageInputReadsLocalFileAsDataURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.png")
	if err := os.WriteFile(path, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	value, err := imageInput(path)
	if err != nil || value != "data:image/png;base64,cG5n" {
		t.Fatalf("imageInput() = %q, %v", value, err)
	}
	if value := imageInputString("https://example.test/image.png"); value != "https://example.test/image.png" {
		t.Fatalf("URL changed: %q", value)
	}
}

func imageInputString(value string) string {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "data:") {
		return value
	}
	return ""
}
