package server

import (
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
)

func localDeviceVoices(ctx context.Context) []map[string]any {
	if runtime.GOOS == "windows" {
		return windowsVoices(ctx)
	}
	return macOSVoices(ctx)
}

func macOSVoices(ctx context.Context) []map[string]any {
	output, err := exec.CommandContext(ctx, "say", "-v", "?").Output()
	if err != nil {
		return []map[string]any{}
	}
	voices := []map[string]any{}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		locale := fields[len(fields)-1]
		if !strings.Contains(locale, "_") {
			continue
		}
		name := strings.TrimSpace(strings.TrimSuffix(line, locale))
		parts := strings.SplitN(locale, "_", 2)
		voices = append(voices, map[string]any{"id": name, "name": name, "locale": locale, "lang": parts[0], "country": parts[1], "gender": ""})
	}
	return voices
}

func windowsVoices(ctx context.Context) []map[string]any {
	script := "$s=New-Object System.Speech.Synthesis.SpeechSynthesizer; $s.GetInstalledVoices() | %% { $v=$_.VoiceInfo; [PSCustomObject]@{Name=$v.Name;Culture=$v.Culture.Name;Gender=$v.Gender} } | ConvertTo-Json -Compress"
	output, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script).Output()
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return []map[string]any{}
	}
	var raw any
	if json.Unmarshal(output, &raw) != nil {
		return []map[string]any{}
	}
	items, ok := raw.([]any)
	if !ok {
		items = []any{raw}
	}
	voices := []map[string]any{}
	for _, item := range items {
		value, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := value["Name"].(string)
		locale, _ := value["Culture"].(string)
		parts := strings.SplitN(locale, "-", 2)
		country := ""
		if len(parts) == 2 {
			country = parts[1]
		}
		voices = append(voices, map[string]any{"id": name, "name": name, "locale": strings.ReplaceAll(locale, "-", "_"), "lang": parts[0], "country": country, "gender": value["Gender"]})
	}
	return voices
}
