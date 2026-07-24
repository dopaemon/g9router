package chatcore

import "testing"

func TestDedupeTools(t *testing.T) {
	result := DedupeTools([]any{map[string]any{"name": "mcp__exa__web_search_exa"}, map[string]any{"name": "WebSearch"}, map[string]any{"name": "Read"}})
	if len(result.Tools) != 2 || len(result.Stripped) != 1 {
		t.Fatal(result)
	}
}
