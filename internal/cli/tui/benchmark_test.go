package tui

import "testing"

func BenchmarkRedactLogText(b *testing.B) {
	value := "bearer secret-token provider=demo api_key=hidden"
	b.ReportAllocs()
	for b.Loop() {
		redactLogText(value)
	}
}

func BenchmarkFormatAPILog(b *testing.B) {
	model := logsModel{ui: &UI{width: 100, height: 30}}
	entry := apiLogEntry{Timestamp: "12:34:56", Provider: "openai-compatible", Model: "gpt-5.4", Status: "200", Input: 1024, Output: 512}
	b.ReportAllocs()
	for b.Loop() {
		model.formatAPILog(entry.Timestamp, entry.Status, entry)
	}
}

func BenchmarkGradientText(b *testing.B) {
	value := "9Router CLI Endpoint & Key"
	b.ReportAllocs()
	for b.Loop() {
		gradientText(value)
	}
}

func BenchmarkFitView(b *testing.B) {
	ui := &UI{height: 24}
	view := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\nline 11\nline 12\nline 13\nline 14\nline 15\nline 16\nline 17\nline 18\nline 19\nline 20\nline 21\nline 22\nline 23\nline 24\nline 25\nline 26\nline 27\nline 28\nline 29\nline 30"
	b.ReportAllocs()
	for b.Loop() {
		ui.fitView(view)
	}
}
