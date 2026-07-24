package chatcore

import "strings"

type Capabilities struct{ Vision, AudioInput, PDF bool }

func StripUnsupported(body map[string]any, format string, caps Capabilities) bool {
	removed := false
	switch format {
	case "openai", "claude":
		messages, _ := body["messages"].([]any)
		last := len(messages) - 1
		for i, raw := range messages {
			message, _ := raw.(map[string]any)
			parts, ok := message["content"].([]any)
			if !ok {
				continue
			}
			out := []any{}
			removedKinds := map[string]bool{}
			for _, partRaw := range parts {
				part, _ := partRaw.(map[string]any)
				kind := required(part, format)
				if kind == "vision" && !caps.Vision || kind == "audio" && !caps.AudioInput || kind == "pdf" && !caps.PDF {
					removed = true
					removedKinds[kind] = true
					continue
				}
				out = append(out, part)
			}
			for kind := range removedKinds {
				out = append(out, map[string]any{"type": "text", "text": placeholder(kind, i == last)})
			}
			message["content"] = out
		}
	}
	return removed
}
func required(part map[string]any, format string) string {
	kind, _ := part["type"].(string)
	if format == "claude" {
		if kind == "image" {
			return "vision"
		}
		if kind == "document" {
			return "pdf"
		}
		return ""
	}
	if kind == "image_url" || kind == "image" {
		return "vision"
	}
	if kind == "input_audio" || kind == "audio_url" {
		return "audio"
	}
	if kind == "file" {
		return "pdf"
	}
	return ""
}
func placeholder(kind string, last bool) string {
	if last {
		switch kind {
		case "vision":
			return "[image omitted: model has no vision support]"
		case "audio":
			return "[audio omitted: model has no audio support]"
		case "pdf":
			return "[file omitted: model has no document support]"
		}
	}
	switch kind {
	case "vision":
		return "[Previous image omitted from context.]"
	case "audio":
		return "[Previous audio omitted from context.]"
	case "pdf":
		return "[Previous file omitted from context.]"
	}
	return strings.TrimSpace(kind)
}
