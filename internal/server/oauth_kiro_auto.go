package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func (s *Server) kiroAutoImportAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "error": err.Error()})
		return
	}
	cachePath := filepath.Join(home, ".aws", "sso", "cache")
	entries, err := os.ReadDir(cachePath)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "error": "AWS SSO cache not found. Please login to Kiro IDE first."})
		return
	}
	var refreshToken, source string
	var tokenData map[string]any
	ordered := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() == "kiro-auth-token.json" {
			ordered = append([]os.DirEntry{entry}, ordered...)
		} else {
			ordered = append(ordered, entry)
		}
	}
	for _, entry := range ordered {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(cachePath, entry.Name()))
		if readErr != nil {
			continue
		}
		var value map[string]any
		if json.Unmarshal(data, &value) != nil {
			continue
		}
		candidate, _ := value["refreshToken"].(string)
		if strings.HasPrefix(candidate, "aorAAAAAG") {
			refreshToken, source, tokenData = candidate, entry.Name(), value
			break
		}
	}
	if refreshToken == "" {
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "error": "Kiro token not found in AWS SSO cache. Please login to Kiro IDE first."})
		return
	}
	result := map[string]any{"found": true, "refreshToken": refreshToken, "source": source, "clientId": nil, "clientSecret": nil, "region": tokenData["region"], "authMethod": tokenData["authMethod"], "profileArn": nil}
	if hash, ok := tokenData["clientIdHash"].(string); ok && hash != "" {
		if data, readErr := os.ReadFile(filepath.Join(cachePath, hash+".json")); readErr == nil {
			var client map[string]any
			if json.Unmarshal(data, &client) == nil {
				result["clientId"], result["clientSecret"] = client["clientId"], client["clientSecret"]
			}
		}
	}
	profilePaths := []string{filepath.Join(home, ".config", "Kiro", "User", "globalStorage", "kiro.kiroagent", "profile.json")}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			profilePaths = append([]string{filepath.Join(appData, "Kiro", "User", "globalStorage", "kiro.kiroagent", "profile.json")}, profilePaths...)
		}
	}
	for _, path := range profilePaths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		var profile map[string]any
		if json.Unmarshal(data, &profile) == nil {
			if arn, ok := profile["arn"].(string); ok {
				result["profileArn"] = normalizeKiroProfileARN(arn)
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func normalizeKiroProfileARN(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) > 4 && parts[2] == "codewhisperer" {
		parts[3] = "us-east-1"
		return strings.Join(parts, ":")
	}
	return arn
}
