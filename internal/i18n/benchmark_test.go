package i18n

import "testing"

func BenchmarkT(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		T(Vietnamese, "menu.providers")
	}
}

func BenchmarkNormalize(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		Normalize(" VI ")
	}
}
