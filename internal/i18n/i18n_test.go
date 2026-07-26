package i18n

import "testing"

func TestTranslations(t *testing.T) {
	if got := T(English, "menu.providers"); got != "Providers" {
		t.Fatalf("English translation = %q", got)
	}
	if got := T(Vietnamese, "menu.providers"); got != "Nhà cung cấp" {
		t.Fatalf("Vietnamese translation = %q", got)
	}
	if Normalize("fr") != English || Normalize("VI") != Vietnamese {
		t.Fatal("locale normalization failed")
	}
}
