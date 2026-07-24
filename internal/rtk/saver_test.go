package rtk

import (
	"strings"
	"testing"
)

func TestCompressPreservesShortTextAndCompactsLongText(t *testing.T) {
	if Compress(" hi ") != "hi" {
		t.Fatal(Compress(" hi "))
	}
	if len(Compress(strings.Repeat("word   ", 100))) >= len(strings.Repeat("word   ", 100)) {
		t.Fatal("not compressed")
	}
}
