package rtk

import (
	"regexp"
	"strings"
)

var whitespace = regexp.MustCompile(`\s+`)

func Compress(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 160 {
		return value
	}
	return whitespace.ReplaceAllString(value, " ")
}

func CompressMessages(messages []any) {
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role, _ := message["role"].(string)
		if role != "tool" && role != "user" {
			continue
		}
		if content, ok := message["content"].(string); ok {
			message["content"] = Compress(content)
		}
	}
}
