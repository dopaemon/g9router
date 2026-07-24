package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (s *Server) jcodeSettingsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.jcodeStatus(w)
	case http.MethodPost:
		s.jcodeConfigure(w, r)
	case http.MethodDelete:
		s.jcodeRemove(w)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func jcodePaths() (string, string, string) {
	home, _ := os.UserHomeDir()
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(home, ".jcode", "config.toml"), filepath.Join(xdg, "jcode", "provider-9router.env"), home
}

func jcodeInstalled() bool {
	if _, err := exec.LookPath("jcode"); err == nil {
		return true
	}
	config, _, _ := jcodePaths()
	_, err := os.Stat(filepath.Dir(config))
	return err == nil
}

func readText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func jcodeHas9Router(config string) bool {
	return strings.Contains(config, "[providers.9router]") || strings.Contains(config, "base_url = \"http://localhost:20128")
}

func (s *Server) jcodeStatus(w http.ResponseWriter) {
	configPath, _, _ := jcodePaths()
	if !jcodeInstalled() {
		writeJSON(w, http.StatusOK, map[string]any{"installed": false, "message": "jcode not installed. Install via: curl -fsSL https://raw.githubusercontent.com/1jehuang/jcode/master/scripts/install.sh | bash"})
		return
	}
	config := readText(configPath)
	writeJSON(w, http.StatusOK, map[string]any{"installed": true, "config": config, "has9Router": jcodeHas9Router(config), "configPath": configPath})
}

func (s *Server) jcodeConfigure(w http.ResponseWriter, r *http.Request) {
	var input struct {
		BaseURL string   `json:"baseUrl"`
		APIKey  string   `json:"apiKey"`
		Models  []string `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil || strings.TrimSpace(input.BaseURL) == "" || strings.TrimSpace(input.APIKey) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "baseUrl and apiKey are required"})
		return
	}
	configPath, envPath, _ := jcodePaths()
	baseURL := strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL += "/v1"
	}
	model := "cc/claude-opus-4-7"
	if len(input.Models) > 0 && strings.TrimSpace(input.Models[0]) != "" {
		model = strings.TrimSpace(input.Models[0])
	}
	config := readText(configPath)
	config = upsertJcodeProvider(config, baseURL, model)
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := upsertJcodeEnv(envPath, input.APIKey); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "jcode configured successfully. Use: jcode --provider-profile 9router", "configPath": configPath})
}

func upsertJcodeProvider(config, baseURL, model string) string {
	block := fmt.Sprintf("[providers.9router]\ntype = \"openai-compatible\"\nbase_url = \"%s\"\nauth = \"bearer\"\napi_key_env = \"JCODE_9ROUTER_API_KEY\"\nenv_file = \"provider-9router.env\"\ndefault_model = \"%s\"\nrequires_api_key = true\n", baseURL, model)
	if start := strings.Index(config, "[providers.9router]"); start >= 0 {
		rest := config[start:]
		if next := strings.Index(rest[1:], "\n["); next >= 0 {
			return config[:start] + block + rest[next+1:]
		}
		return config[:start] + block
	}
	if config != "" && !strings.HasSuffix(config, "\n") {
		config += "\n"
	}
	return config + "\n" + block
}

func upsertJcodeEnv(path, apiKey string) error {
	lines := strings.Split(readText(path), "\n")
	found := false
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "JCODE_9ROUTER_API_KEY=") {
			lines[index], found = "JCODE_9ROUTER_API_KEY=\""+apiKey+"\"", true
		}
	}
	if !found {
		lines = append(lines, "JCODE_9ROUTER_API_KEY=\""+apiKey+"\"")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("# jcode provider environment variables\n"+strings.Join(lines, "\n")), 0600)
}

func (s *Server) jcodeRemove(w http.ResponseWriter) {
	configPath, envPath, _ := jcodePaths()
	config := readText(configPath)
	if start := strings.Index(config, "[providers.9router]"); start >= 0 {
		rest := config[start:]
		if next := strings.Index(rest[1:], "\n["); next >= 0 {
			config = config[:start] + rest[next+1:]
		} else {
			config = config[:start]
		}
		if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	env := readText(envPath)
	kept := make([]string, 0)
	for _, line := range strings.Split(env, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "JCODE_9ROUTER_API_KEY=") {
			kept = append(kept, line)
		}
	}
	if err := os.WriteFile(envPath, []byte(strings.Join(kept, "\n")), 0600); err != nil && !os.IsNotExist(err) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"success": "true", "message": "9router configuration removed from jcode"})
}
