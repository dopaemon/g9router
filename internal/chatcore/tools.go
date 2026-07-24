package chatcore

import "strings"

type ToolDedupResult struct {
	Tools    []any
	Stripped []string
}

func DedupeTools(tools []any) ToolDedupResult {
	if len(tools) == 0 {
		return ToolDedupResult{Tools: tools}
	}
	names := make([]string, len(tools))
	for i, raw := range tools {
		tool, _ := raw.(map[string]any)
		names[i] = toolName(tool)
	}
	strip := map[string]bool{}
	has := func(trigger string) bool {
		for _, name := range names {
			if name == trigger || strings.HasPrefix(trigger, "^") && strings.HasPrefix(name, strings.TrimPrefix(trigger, "^")) {
				return true
			}
		}
		return false
	}
	if has("mcp__exa__web_search_exa") || has("mcp__exa__web_fetch_exa") || has("mcp__tavily__tavily_search") || has("mcp__tavily__tavily_extract") {
		strip["WebSearch"] = true
		strip["WebFetch"] = true
		strip["mcp__workspace__web_fetch"] = true
	}
	if has("^mcp__browsermcp__") {
		for _, name := range names {
			if strings.HasPrefix(name, "mcp__Claude_in_Chrome__") {
				strip[name] = true
			}
		}
	}
	result := make([]any, 0, len(tools))
	stripped := []string{}
	for i, tool := range tools {
		if strip[names[i]] {
			stripped = append(stripped, names[i])
			continue
		}
		result = append(result, tool)
	}
	return ToolDedupResult{Tools: result, Stripped: stripped}
}
func toolName(tool map[string]any) string {
	if name, ok := tool["name"].(string); ok {
		return name
	}
	if function, ok := tool["function"].(map[string]any); ok {
		if name, ok := function["name"].(string); ok {
			return name
		}
	}
	return ""
}
