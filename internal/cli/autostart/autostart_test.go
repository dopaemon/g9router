package autostart

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAutostartConfigPathUsesPlatformConvention(t *testing.T) {
	path, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) == "" {
		t.Fatal(path)
	}
	if runtime.GOOS == "linux" && filepath.Base(path) != "9router.desktop" {
		t.Fatal(path)
	}
}

func TestDisableMissingEntryIsSafe(t *testing.T) {
	if err := Disable(); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}
