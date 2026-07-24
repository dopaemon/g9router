package server

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func localDeviceTTSAPI(w http.ResponseWriter, r *http.Request, input map[string]any) bool {
	model, _ := input["model"].(string)
	if model != "local-device" && !strings.HasPrefix(model, "local-device/") {
		return false
	}
	text, _ := input["input"].(string)
	if strings.TrimSpace(text) == "" {
		writeJSON(w, 400, map[string]string{"error": "Missing required field: input"})
		return true
	}
	voice := strings.TrimPrefix(model, "local-device/")
	audio, err := synthesizeLocalDevice(r.Context(), text, voice)
	if err != nil {
		return false
	}
	if input["response_format"] == "json" || r.URL.Query().Get("response_format") == "json" {
		writeJSON(w, 200, map[string]string{"audio": base64.StdEncoding.EncodeToString(audio), "format": "mp3"})
		return true
	}
	w.Header().Set("Content-Type", "audio/mp3")
	w.WriteHeader(200)
	_, _ = w.Write(audio)
	return true
}

func synthesizeLocalDevice(ctx context.Context, text, voice string) ([]byte, error) {
	directory, err := os.MkdirTemp("", "g9router-tts-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	aiff := filepath.Join(directory, "out.aiff")
	mp3 := filepath.Join(directory, "out.mp3")
	args := []string{"-o", aiff, text}
	if voice != "" {
		args = []string{"-v", voice, "-o", aiff, text}
	}
	if output, err := exec.CommandContext(ctx, "say", args...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("local TTS failed: %s", strings.TrimSpace(string(output)))
	}
	if output, err := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", aiff, "-codec:a", "libmp3lame", "-qscale:a", "4", mp3).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("local TTS conversion failed: %s", strings.TrimSpace(string(output)))
	}
	audio, err := os.ReadFile(mp3)
	if err != nil {
		return nil, err
	}
	if len(audio) < 1024 {
		return nil, fmt.Errorf("local TTS returned empty audio")
	}
	return audio, nil
}
