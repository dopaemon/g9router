package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type coworkConfig struct {
	InferenceProvider   string           `json:"inferenceProvider"`
	InferenceGatewayURL string           `json:"inferenceGatewayBaseUrl"`
	InferenceGatewayKey string           `json:"inferenceGatewayApiKey"`
	InferenceModels     []map[string]any `json:"inferenceModels"`
	ManagedMCPServers   []map[string]any `json:"managedMcpServers,omitempty"`
	DeploymentMode      string           `json:"deploymentMode"`
	DisableTelemetry    bool             `json:"disableNonessentialTelemetry"`
	DisableServices     bool             `json:"disableNonessentialServices"`
}

func coworkConfigPath() string {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".config", "Claude-3p")
	if runtime.GOOS == "darwin" {
		root = filepath.Join(home, "Library", "Application Support", "Claude-3p")
	}
	if runtime.GOOS == "windows" {
		root = filepath.Join(os.Getenv("APPDATA"), "Claude-3p")
	}
	return filepath.Join(root, "configLibrary", "g9router.json")
}

func (s *Server) coworkSettingsAPI(w http.ResponseWriter, r *http.Request) {
	path := coworkConfigPath()
	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, map[string]any{"installed": false, "config": nil})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		var config map[string]any
		if json.Unmarshal(data, &config) != nil {
			config = map[string]any{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"installed": true, "config": config, "configPath": path, "has9Router": config["inferenceProvider"] == "gateway"})
	case http.MethodPost:
		var input struct {
			BaseURL       string           `json:"baseUrl"`
			APIKey        string           `json:"apiKey"`
			Models        []string         `json:"models"`
			Plugins       []map[string]any `json:"plugins"`
			LocalPlugins  []string         `json:"localPlugins"`
			CustomPlugins []map[string]any `json:"customPlugins"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&input) != nil || strings.TrimSpace(input.BaseURL) == "" || strings.TrimSpace(input.APIKey) == "" || len(input.Models) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "baseUrl, apiKey, and at least one model are required"})
			return
		}
		models := make([]map[string]any, 0, len(input.Models))
		for _, model := range input.Models {
			if model = strings.TrimSpace(model); model != "" {
				models = append(models, map[string]any{"name": model})
			}
		}
		if len(models) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one model is required"})
			return
		}
		config := map[string]any{"inferenceProvider": "gateway", "inferenceGatewayBaseUrl": strings.TrimRight(strings.TrimSpace(input.BaseURL), "/"), "inferenceGatewayApiKey": input.APIKey, "inferenceModels": models, "deploymentMode": "3p", "disableNonessentialTelemetry": true, "disableNonessentialServices": true}
		if len(input.Plugins) > 0 {
			config["managedMcpServers"] = input.Plugins
		}
		if len(input.LocalPlugins) > 0 {
			config["localPlugins"] = input.LocalPlugins
		}
		if len(input.CustomPlugins) > 0 {
			config["customPlugins"] = input.CustomPlugins
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		data, _ := json.MarshalIndent(config, "", "  ")
		if err := os.WriteFile(path, data, 0600); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "configPath": path, "message": "Cowork settings applied. Restart Claude Desktop."})
	case http.MethodDelete:
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Cowork config reset"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
