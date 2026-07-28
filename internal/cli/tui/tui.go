package tui

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"g9router/internal/i18n"
	"g9router/internal/providers"
)

type UI struct {
	BaseURL  string
	In       io.Reader
	Out      io.Writer
	Client   *http.Client
	Locale   string
	width    int
	height   int
	forceHuh bool
}

func Run(baseURL string, in io.Reader, out io.Writer) error {
	ui := &UI{BaseURL: strings.TrimRight(baseURL, "/"), In: in, Out: out, Client: http.DefaultClient, Locale: i18n.Normalize(os.Getenv("G9ROUTER_LOCALE")), forceHuh: true}
	EnableColors(out)
	reader := bufio.NewReader(in)
	return ui.huhMenu(reader)
}

func (ui *UI) t(key string) string { return i18n.T(ui.Locale, key) }

const cliBanner = ` ██████╗  █████╗ ██████╗  ██████╗ ██╗   ██╗████████╗███████╗██████╗
██╔════╝ ██╔══██╗██╔══██╗██╔═══██╗██║   ██║╚══██╔══╝██╔════╝██╔══██╗
██║  ███╗╚██████║██████╔╝██║   ██║██║   ██║   ██║   █████╗  ██████╔╝
██║   ██║ ╚═══██║██╔══██╗██║   ██║██║   ██║   ██║   ██╔══╝  ██╔══██╗
╚██████╔╝ █████╔╝██║  ██║╚██████╔╝╚██████╔╝   ██║   ███████╗██║  ██║
 ╚═════╝  ╚════╝ ╚═╝  ╚═╝ ╚═════╝  ╚═════╝    ╚═╝   ╚══════╝╚═╝  ╚═╝`

func (ui *UI) showJSON(reader *bufio.Reader, path string) error {
	request, err := http.NewRequest(http.MethodGet, ui.BaseURL+path, nil)
	if err != nil {
		return err
	}
	response, err := ui.Client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return err
	}
	var value any
	if json.Unmarshal(data, &value) == nil {
		pretty, _ := json.MarshalIndent(redactSecrets(value), "", "  ")
		fmt.Fprintln(ui.Out, string(pretty))
	} else {
		fmt.Fprintln(ui.Out, string(data))
	}
	fmt.Fprint(ui.Out, "Press Enter to continue...")
	_, _ = reader.ReadString('\n')
	if response.StatusCode >= 400 {
		return fmt.Errorf("HTTP %s", response.Status)
	}
	return nil
}

type apiKey struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Key      string `json:"key"`
	IsActive bool   `json:"isActive"`
}

type provider struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey"`
	APIType string `json:"apiType"`
	OAuthID string `json:"oauthId"`
	Enabled bool   `json:"enabled"`
}

type providersResponse struct {
	Connections []provider `json:"connections"`
}

type combo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Models []any  `json:"models"`
	Kind   string `json:"kind,omitempty"`
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "••••"
	}
	return value[:4] + "…" + value[len(value)-4:]
}

func redactSecrets(value any) any {
	switch value := value.(type) {
	case map[string]any:
		for key, item := range value {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "key") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") {
				if text, ok := item.(string); ok {
					value[key] = maskSecret(text)
					continue
				}
			}
			value[key] = redactSecrets(item)
		}
	case []any:
		for index := range value {
			value[index] = redactSecrets(value[index])
		}
	}
	return value
}

func (ui *UI) oauth(reader *bufio.Reader) error {
	for {
		var credentials any
		if err := ui.request(http.MethodGet, "/api/oauth", nil, &credentials); err != nil {
			return err
		}
		fmt.Fprintln(ui.Out, "\n"+ui.t("oauth.title"))
		pretty, _ := json.MarshalIndent(redactSecrets(credentials), "", "  ")
		fmt.Fprintln(ui.Out, string(pretty))
		var line string
		var err error
		if ui.huhMode() {
			choices := []string{ui.t("oauth.login"), ui.t("oauth.loginCodex"), ui.t("oauth.importToken"), ui.t("oauth.importJSON"), ui.t("oauth.delete"), ui.t("oauth.back")}
			choice, choiceErr := ui.huhChoice(ui.t("oauth.action"), choices)
			if choiceErr != nil {
				return choiceErr
			}
			line = map[string]string{ui.t("oauth.login"): "1", ui.t("oauth.loginCodex"): "l", ui.t("oauth.importToken"): "c", ui.t("oauth.importJSON"): "i", ui.t("oauth.delete"): "d", ui.t("oauth.back"): "b"}[choice]
		} else {
			fmt.Fprintf(ui.Out, "1. %s  l. %s  c. %s  i. %s  d. %s  b. %s\n", ui.t("oauth.login"), ui.t("oauth.loginCodex"), ui.t("oauth.importToken"), ui.t("oauth.importJSON"), ui.t("oauth.delete"), ui.t("oauth.back"))
			fmt.Fprint(ui.Out, "Select action: ")
			line, err = reader.ReadString('\n')
			if err != nil && len(line) == 0 {
				return err
			}
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "b", "0":
			return nil
		case "d":
			id := ""
			if ui.huhMode() {
				id, err = ui.huhValue(ui.t("oauth.credentialID"), "", false)
			} else {
				fmt.Fprint(ui.Out, ui.t("oauth.credentialID")+": ")
				id, err = reader.ReadString('\n')
			}
			if err != nil {
				return err
			}
			if ui.huhMode() {
				ok, confirmErr := ui.huhConfirm(ui.t("oauth.deleteConfirm"), false)
				if confirmErr != nil {
					return confirmErr
				}
				if !ok {
					continue
				}
			}
			if err := ui.request(http.MethodDelete, "/api/oauth/"+strings.TrimSpace(id), nil, nil); err != nil {
				return err
			}
		case "c":
			var token, name string
			if ui.huhMode() {
				token, err = ui.huhValue(ui.t("oauth.accessToken"), "", true)
				if err == nil {
					name, err = ui.huhValue(ui.t("oauth.connectionName"), "", false)
				}
			} else {
				fmt.Fprint(ui.Out, ui.t("oauth.accessToken")+": ")
				token, err = reader.ReadString('\n')
				if err == nil {
					fmt.Fprint(ui.Out, ui.t("oauth.connectionName")+": ")
					name, err = reader.ReadString('\n')
				}
			}
			if err != nil {
				return err
			}
			var result any
			if err := ui.request(http.MethodPost, "/api/oauth/codex/import-token", map[string]string{"accessToken": strings.TrimSpace(token), "name": strings.TrimSpace(name)}, &result); err != nil {
				return err
			}
			pretty, _ := json.MarshalIndent(redactSecrets(result), "", "  ")
			fmt.Fprintln(ui.Out, string(pretty))
		case "l":
			if err := ui.loginCodex(); err != nil {
				return err
			}
		case "1", "o":
			if err := ui.loginOAuthProvider(reader); err != nil {
				return err
			}
		case "i":
			fmt.Fprint(ui.Out, ui.t("oauth.importEndpoint")+": ")
			path, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			fmt.Fprint(ui.Out, ui.t("oauth.jsonBody")+": ")
			body, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			var payload any
			if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &payload); err != nil {
				fmt.Fprintln(ui.Out, ui.t("oauth.invalidJSON"))
				continue
			}
			var result any
			if err := ui.request(http.MethodPost, strings.TrimSpace(path), payload, &result); err != nil {
				return err
			}
			pretty, _ := json.MarshalIndent(redactSecrets(result), "", "  ")
			fmt.Fprintln(ui.Out, string(pretty))
		default:
			fmt.Fprintln(ui.Out, ui.t("oauth.invalidSelection"))
		}
	}
}

func (ui *UI) loginOAuthProvider(reader *bufio.Reader) error {
	providers := []string{"claude", "xai", "gemini-cli", "antigravity", "cline", "clinepass", "kimchi", "iflow"}
	if ui.huhMode() {
		back := ui.t("oauth.back")
		providers = append(providers, back)
		choice, err := ui.huhChoice(ui.t("oauth.provider"), providers)
		if err != nil || choice == back {
			return err
		}
		return ui.loginBrowserOAuth(reader, choice)
	}
	fmt.Fprintln(ui.Out, "\n"+ui.t("oauth.provider"))
	for index, provider := range providers {
		fmt.Fprintf(ui.Out, "%d. %s\n", index+1, provider)
	}
	fmt.Fprint(ui.Out, ui.t("oauth.providerBack")+": ")
	value, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	index, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || index == 0 {
		return nil
	}
	if index < 1 || index > len(providers) {
		return errors.New(ui.t("oauth.invalidProvider"))
	}
	return ui.loginBrowserOAuth(reader, providers[index-1])
}

func (ui *UI) loginBrowserOAuth(reader *bufio.Reader, provider string) error {
	redirectURI := "http://localhost:8080/callback"
	var authData struct {
		AuthURL      string `json:"authUrl"`
		State        string `json:"state"`
		CodeVerifier string `json:"codeVerifier"`
	}
	path := "/api/oauth/" + url.PathEscape(provider) + "/authorize?redirect_uri=" + url.QueryEscape(redirectURI)
	if err := ui.request(http.MethodGet, path, nil, &authData); err != nil {
		return err
	}
	if authData.AuthURL == "" {
		return fmt.Errorf("%s authorization URL is unavailable", provider)
	}
	fmt.Fprintln(ui.Out, "Opening OAuth login in browser...")
	if err := openURL(authData.AuthURL); err != nil {
		fmt.Fprintf(ui.Out, "Open this URL manually:\n%s\n", authData.AuthURL)
	}
	fmt.Fprint(ui.Out, "Paste callback URL: ")
	callback, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	callback = strings.TrimSpace(callback)
	parsed, err := url.Parse(callback)
	if err != nil {
		return fmt.Errorf("invalid callback URL: %w", err)
	}
	code := parsed.Query().Get("code")
	if code == "" {
		code = callback
	}
	payload := map[string]string{"code": code, "redirect_uri": redirectURI}
	if authData.CodeVerifier != "" {
		payload["code_verifier"] = authData.CodeVerifier
	}
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	exchange := "/api/oauth/" + url.PathEscape(provider) + "/exchange"
	if err := ui.request(http.MethodPost, exchange, payload, &result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("%s OAuth login failed: %s", provider, result.Error)
	}
	fmt.Fprintf(ui.Out, "%s OAuth login successful.\n", provider)
	return nil
}

func (ui *UI) loginCodex() error {
	base, err := url.Parse(ui.BaseURL)
	if err != nil {
		return err
	}
	redirectURI := "http://localhost:1455/auth/callback"
	port := base.Port()
	if port == "" {
		port = "20128"
	}
	var authData struct {
		AuthURL      string `json:"authUrl"`
		State        string `json:"state"`
		CodeVerifier string `json:"codeVerifier"`
	}
	authorizePath := "/api/oauth/codex/authorize?redirect_uri=" + url.QueryEscape(redirectURI)
	if err := ui.request(http.MethodGet, authorizePath, nil, &authData); err != nil {
		return err
	}
	if authData.AuthURL == "" || authData.State == "" || authData.CodeVerifier == "" {
		return fmt.Errorf("Codex authorization data is incomplete")
	}
	var proxyData struct {
		Success    bool   `json:"success"`
		ServerSide bool   `json:"serverSide"`
		Reason     string `json:"reason"`
	}
	proxyPath := "/api/oauth/codex/start-proxy?app_port=" + url.QueryEscape(port) +
		"&state=" + url.QueryEscape(authData.State) +
		"&code_verifier=" + url.QueryEscape(authData.CodeVerifier) +
		"&redirect_uri=" + url.QueryEscape(redirectURI)
	if err := ui.request(http.MethodGet, proxyPath, nil, &proxyData); err != nil {
		return err
	}
	if !proxyData.Success || !proxyData.ServerSide {
		if proxyData.Reason == "" {
			proxyData.Reason = "server-side OAuth proxy unavailable"
		}
		return fmt.Errorf("Codex login unavailable: %s", proxyData.Reason)
	}
	fmt.Fprintln(ui.Out, "Opening Codex login in browser...")
	if err := openURL(authData.AuthURL); err != nil {
		fmt.Fprintf(ui.Out, "Open this URL manually:\n%s\n", authData.AuthURL)
	}
	fmt.Fprintln(ui.Out, "Waiting for Codex callback...")
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		var status struct {
			Status string `json:"status"`
			Email  string `json:"email"`
			Error  string `json:"error"`
		}
		path := "/api/oauth/codex/poll-status?state=" + url.QueryEscape(authData.State)
		if err := ui.request(http.MethodGet, path, nil, &status); err != nil {
			return err
		}
		if status.Status == "done" {
			if status.Email != "" {
				fmt.Fprintf(ui.Out, "Codex login successful: %s\n", status.Email)
			} else {
				fmt.Fprintln(ui.Out, "Codex login successful.")
			}
			return nil
		}
		if status.Status == "error" {
			return fmt.Errorf("Codex login failed: %s", status.Error)
		}
		time.Sleep(1500 * time.Millisecond)
	}
	return fmt.Errorf("Codex login timed out")
}

func openURL(target string) error {
	command := "xdg-open"
	args := []string{target}
	switch runtime.GOOS {
	case "darwin":
		command = "open"
	case "windows":
		command = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", target}
	}
	return exec.Command(command, args...).Start()
}

func (ui *UI) cliTools(reader *bufio.Reader) error {
	paths := map[string]string{"claude": "/api/cli-tools/claude-settings", "codex": "/api/cli-tools/codex-settings", "opencode": "/api/cli-tools/opencode-settings", "copilot": "/api/cli-tools/copilot-settings", "droid": "/api/cli-tools/droid-settings", "cline": "/api/cli-tools/cline-settings", "kilo": "/api/cli-tools/kilo-settings", "openclaw": "/api/cli-tools/openclaw-settings", "deepseek-tui": "/api/cli-tools/deepseek-tui-settings", "grok-build": "/api/cli-tools/grok-build-settings", "hermes": "/api/cli-tools/hermes-settings", "jcode": "/api/cli-tools/jcode-settings", "cowork": "/api/cli-tools/cowork-settings"}
	for {
		var statuses map[string]any
		if err := ui.request(http.MethodGet, "/api/cli-tools/all-statuses", nil, &statuses); err != nil {
			return err
		}
		fmt.Fprintln(ui.Out, "\n"+ui.t("legacy.cliTools"))
		keys := make([]string, 0, len(statuses))
		for name := range statuses {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		for i, name := range keys {
			fmt.Fprintf(ui.Out, "%d. %s: %v\n", i+1, name, statuses[name])
		}
		var line string
		var err error
		if ui.huhMode() {
			choices := []string{ui.t("legacy.quickSetup"), ui.t("legacy.claudeModel"), ui.t("legacy.showSettings"), ui.t("legacy.applyJSON"), ui.t("legacy.reset"), ui.t("oauth.back")}
			choice, choiceErr := ui.huhChoice(ui.t("legacy.cliAction"), choices)
			if choiceErr != nil {
				return choiceErr
			}
			line = map[string]string{ui.t("legacy.quickSetup"): "q", ui.t("legacy.claudeModel"): "m", ui.t("legacy.showSettings"): "s", ui.t("legacy.applyJSON"): "a", ui.t("legacy.reset"): "r", ui.t("oauth.back"): "b"}[choice]
		} else {
			fmt.Fprintf(ui.Out, "q. %s  m. %s  s. %s  a. %s  r. %s  b. %s\n", ui.t("legacy.quickSetup"), ui.t("legacy.claudeModel"), ui.t("legacy.showSettings"), ui.t("legacy.applyJSON"), ui.t("legacy.reset"), ui.t("oauth.back"))
			fmt.Fprint(ui.Out, ui.t("legacy.selectAction")+": ")
			line, err = reader.ReadString('\n')
			if err != nil && len(line) == 0 {
				return err
			}
		}
		action := strings.ToLower(strings.TrimSpace(line))
		if action == "b" || action == "0" {
			return nil
		}
		if action == "q" {
			if err := ui.quickSetup(reader); err != nil {
				return err
			}
			continue
		}
		if action == "m" {
			if err := ui.selectClaudeModel(reader); err != nil {
				return err
			}
			continue
		}
		if action != "s" && action != "a" && action != "r" {
			fmt.Fprintln(ui.Out, ui.t("legacy.invalidSelection"))
			continue
		}
		var index int
		if ui.huhMode() {
			index, err = ui.huhNumber(ui.t("legacy.chooseTool"), keys)
		} else {
			fmt.Fprint(ui.Out, ui.t("legacy.toolNumber")+": ")
			line, err = reader.ReadString('\n')
			if err == nil {
				index, err = strconv.Atoi(strings.TrimSpace(line))
			}
		}
		if err != nil {
			return err
		}
		if err != nil || index < 1 || index > len(keys) {
			fmt.Fprintln(ui.Out, ui.t("legacy.invalidToolNumber"))
			continue
		}
		path := paths[keys[index-1]]
		if path == "" {
			fmt.Fprintln(ui.Out, ui.t("legacy.toolUnavailable"))
			continue
		}
		if action == "s" {
			if err := ui.showJSON(reader, path); err != nil {
				return err
			}
			continue
		}
		if action == "r" {
			if ui.huhMode() {
				ok, confirmErr := ui.huhConfirm("Reset this tool settings?", false)
				if confirmErr != nil {
					return confirmErr
				}
				if !ok {
					continue
				}
			}
			if err := ui.request(http.MethodDelete, path, nil, nil); err != nil {
				return err
			}
			continue
		}
		fmt.Fprint(ui.Out, ui.t("legacy.jsonBody")+": ")
		body, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		var payload any
		if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &payload); err != nil {
			fmt.Fprintln(ui.Out, ui.t("oauth.invalidJSON"))
			continue
		}
		if err := ui.request(http.MethodPost, path, payload, nil); err != nil {
			return err
		}
	}
}

func (ui *UI) selectClaudeModel(reader *bufio.Reader) error {
	fmt.Fprint(ui.Out, ui.t("legacy.modelType")+": ")
	typeLine, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	modelType := strings.ToLower(strings.TrimSpace(typeLine))
	envKey := map[string]string{
		"sonnet": "ANTHROPIC_DEFAULT_SONNET_MODEL",
		"opus":   "ANTHROPIC_DEFAULT_OPUS_MODEL",
		"haiku":  "ANTHROPIC_DEFAULT_HAIKU_MODEL",
	}[modelType]
	if envKey == "" {
		fmt.Fprintln(ui.Out, ui.t("legacy.invalidModelType"))
		return nil
	}
	fmt.Fprint(ui.Out, ui.t("legacy.model")+": ")
	model, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	return ui.request(http.MethodPost, "/api/cli-tools/claude-settings", map[string]any{"env": map[string]string{envKey: model}}, nil)
}

func (ui *UI) quickSetup(reader *bufio.Reader) error {
	var payload struct {
		Keys []apiKey `json:"keys"`
	}
	if err := ui.request(http.MethodGet, "/api/keys", nil, &payload); err != nil {
		return err
	}
	if len(payload.Keys) == 0 || strings.TrimSpace(payload.Keys[0].Key) == "" {
		fmt.Fprintln(ui.Out, ui.t("legacy.noAPIKeys"))
		return nil
	}
	fmt.Fprint(ui.Out, ui.t("legacy.quickTool")+": ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	tool := strings.ToLower(strings.TrimSpace(line))
	key := payload.Keys[0].Key
	switch tool {
	case "claude":
		return ui.request(http.MethodPost, "/api/cli-tools/claude-settings", map[string]any{
			"env": map[string]string{
				"ANTHROPIC_BASE_URL":             strings.TrimRight(ui.BaseURL, "/") + "/v1",
				"ANTHROPIC_AUTH_TOKEN":           key,
				"API_TIMEOUT_MS":                 "600000",
				"ANTHROPIC_DEFAULT_SONNET_MODEL": "cc/claude-sonnet-4-5-20250929",
				"ANTHROPIC_DEFAULT_OPUS_MODEL":   "cc/claude-opus-4-5-20251101",
				"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "cc/claude-haiku-4-5-20251001",
			},
		}, nil)
	case "codex", "droid", "openclaw", "hermes", "copilot", "cline", "kilo", "deepseek-tui", "grok-build", "jcode", "cowork":
		fmt.Fprint(ui.Out, ui.t("legacy.model")+": ")
		model, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		model = strings.TrimSpace(model)
		if model == "" {
			model = "cx/claude-sonnet-4-5-20250929"
		}
		path := "/api/cli-tools/" + tool + "-settings"
		return ui.request(http.MethodPost, path, map[string]any{"baseUrl": strings.TrimRight(ui.BaseURL, "/") + "/v1", "apiKey": key, "model": model, "models": []string{model}, "activeModel": model}, nil)
	case "opencode":
		fmt.Fprint(ui.Out, ui.t("legacy.model")+": ")
		model, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		model = strings.TrimSpace(model)
		if model == "" {
			model = "cc/claude-sonnet-4-5-20250929"
		}
		return ui.request(http.MethodPost, "/api/cli-tools/opencode-settings", map[string]any{"baseUrl": strings.TrimRight(ui.BaseURL, "/") + "/v1", "apiKey": key, "models": []string{model}, "activeModel": model, "subagentModel": model}, nil)
	default:
		fmt.Fprintln(ui.Out, ui.t("legacy.unsupportedQuick"))
		return nil
	}
}

func (ui *UI) settings(reader *bufio.Reader) error {
	for {
		var values map[string]any
		if err := ui.request(http.MethodGet, "/api/settings", nil, &values); err != nil {
			return err
		}
		var tunnel map[string]any
		_ = ui.request(http.MethodGet, "/api/tunnel/status", nil, &tunnel)
		fmt.Fprintf(ui.Out, "\n%s\nRTK: %v  Headroom: %v  Tunnel: %v\n", ui.t("legacy.settings"), values["rtkEnabled"] != false, values["headroomEnabled"] == true, tunnel["enabled"] == true)
		var line string
		var err error
		if ui.huhMode() {
			choices := []string{ui.t("legacy.toggleRTK"), ui.t("legacy.toggleHeadroom"), ui.t("legacy.enableTunnel"), ui.t("legacy.disableTunnel"), ui.t("legacy.resetAuth"), ui.t("legacy.resetPassword"), ui.t("oauth.back")}
			choice, choiceErr := ui.huhChoice(ui.t("legacy.settingsAction"), choices)
			if choiceErr != nil {
				return choiceErr
			}
			line = map[string]string{ui.t("legacy.toggleRTK"): "1", ui.t("legacy.toggleHeadroom"): "2", ui.t("legacy.enableTunnel"): "3", ui.t("legacy.disableTunnel"): "4", ui.t("legacy.resetAuth"): "5", ui.t("legacy.resetPassword"): "6", ui.t("oauth.back"): "b"}[choice]
		} else {
			fmt.Fprintf(ui.Out, "1. %s  2. %s  3. %s  4. %s  5. %s  6. %s  b. %s\n", ui.t("legacy.toggleRTK"), ui.t("legacy.toggleHeadroom"), ui.t("legacy.tunnelOn"), ui.t("legacy.tunnelOff"), ui.t("legacy.resetAuth"), ui.t("legacy.resetPassword"), ui.t("oauth.back"))
			fmt.Fprint(ui.Out, ui.t("legacy.selectAction")+": ")
			line, err = reader.ReadString('\n')
			if err != nil && len(line) == 0 {
				return err
			}
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "b", "0":
			return nil
		case "1":
			if err := ui.request(http.MethodPut, "/api/settings", map[string]bool{"rtkEnabled": values["rtkEnabled"] == false}, nil); err != nil {
				return err
			}
		case "2":
			if err := ui.request(http.MethodPut, "/api/settings", map[string]bool{"headroomEnabled": values["headroomEnabled"] != true}, nil); err != nil {
				return err
			}
		case "3":
			if err := ui.request(http.MethodPost, "/api/tunnel/enable", nil, nil); err != nil {
				return err
			}
		case "4":
			ok, confirmErr := ui.huhConfirm(ui.t("legacy.disableTunnelConfirm"), false)
			if confirmErr != nil {
				return confirmErr
			}
			if !ok {
				continue
			}
			if err := ui.request(http.MethodPost, "/api/tunnel/disable", nil, nil); err != nil {
				return err
			}
		case "5":
			ok, confirmErr := ui.huhConfirm(ui.t("legacy.authConfirm"), false)
			if confirmErr != nil {
				return confirmErr
			}
			if !ok {
				continue
			}
			if err := ui.request(http.MethodPut, "/api/settings", map[string]string{"authMode": "password"}, nil); err != nil {
				return err
			}
		case "6":
			ok, confirmErr := ui.huhConfirm(ui.t("legacy.passwordConfirm"), false)
			if confirmErr != nil {
				return confirmErr
			}
			if !ok {
				continue
			}
			if err := ui.request(http.MethodPost, "/api/auth/reset-password", nil, nil); err != nil {
				return err
			}
		default:
			fmt.Fprintln(ui.Out, ui.t("legacy.invalidSelection"))
		}
	}
}

func (ui *UI) combos(reader *bufio.Reader) error {
	for {
		var payload struct {
			Combos []combo `json:"combos"`
		}
		if err := ui.request(http.MethodGet, "/api/combos", nil, &payload); err != nil {
			return err
		}
		fmt.Fprintln(ui.Out, "\n"+ui.t("legacy.combos"))
		for i, item := range payload.Combos {
			fmt.Fprintf(ui.Out, "%d. %s: %v\n", i+1, item.Name, item.Models)
		}
		var action string
		var err error
		if ui.huhMode() {
			choices := []string{ui.t("legacy.create"), ui.t("legacy.edit"), ui.t("legacy.delete"), ui.t("oauth.back")}
			choice, choiceErr := ui.huhChoice(ui.t("legacy.comboAction"), choices)
			if choiceErr != nil {
				return choiceErr
			}
			action = map[string]string{ui.t("legacy.create"): "a", ui.t("legacy.edit"): "e", ui.t("legacy.delete"): "d", ui.t("oauth.back"): "b"}[choice]
		} else {
			fmt.Fprintf(ui.Out, "a. %s  e. %s  d. %s  b. %s\n", ui.t("legacy.create"), ui.t("legacy.edit"), ui.t("legacy.delete"), ui.t("oauth.back"))
			fmt.Fprint(ui.Out, ui.t("legacy.selectAction")+": ")
			line, readErr := reader.ReadString('\n')
			if readErr != nil && len(line) == 0 {
				return readErr
			}
			action = strings.ToLower(strings.TrimSpace(line))
		}
		if action == "b" || action == "0" {
			return nil
		}
		if action == "a" {
			if err := ui.promptCombo(&combo{}, false); err != nil {
				return err
			}
			continue
		}
		if action != "e" && action != "d" || len(payload.Combos) == 0 {
			fmt.Fprintln(ui.Out, "Invalid selection")
			continue
		}
		var index int
		if ui.huhMode() {
			labels := make([]string, 0, len(payload.Combos))
			for _, item := range payload.Combos {
				labels = append(labels, item.Name)
			}
			index, err = ui.huhNumber(ui.t("legacy.chooseCombo"), labels)
		} else {
			fmt.Fprint(ui.Out, ui.t("legacy.comboNumber")+": ")
			value, readErr := reader.ReadString('\n')
			if readErr != nil {
				return readErr
			}
			index, err = strconv.Atoi(strings.TrimSpace(value))
		}
		if err != nil {
			return err
		}
		if err != nil || index < 1 || index > len(payload.Combos) {
			fmt.Fprintln(ui.Out, ui.t("legacy.invalidComboNumber"))
			continue
		}
		item := payload.Combos[index-1]
		if action == "d" {
			if ui.huhMode() {
				ok, confirmErr := ui.huhConfirm(ui.t("legacy.deleteComboConfirm"), false)
				if confirmErr != nil {
					return confirmErr
				}
				if !ok {
					continue
				}
			}
			if err := ui.request(http.MethodDelete, "/api/combos/"+item.ID, nil, nil); err != nil {
				return err
			}
			continue
		}
		if ui.huhMode() {
			if err := ui.promptCombo(&item, true); err != nil {
				return err
			}
		} else {
			fmt.Fprint(ui.Out, ui.t("legacy.newName")+" ("+fmt.Sprintf(ui.t("legacy.keepEnter"), item.Name)+"): ")
			value, readErr := reader.ReadString('\n')
			if readErr != nil {
				return readErr
			}
			if strings.TrimSpace(value) != "" {
				item.Name = strings.TrimSpace(value)
			}
			fmt.Fprint(ui.Out, ui.t("legacy.modelsKeep")+": ")
			value, readErr = reader.ReadString('\n')
			if readErr != nil {
				return readErr
			}
			if strings.TrimSpace(value) != "" {
				item.Models = nil
				for _, model := range strings.Split(strings.TrimSpace(value), ",") {
					if strings.TrimSpace(model) != "" {
						item.Models = append(item.Models, strings.TrimSpace(model))
					}
				}
			}
			if err := ui.request(http.MethodPut, "/api/combos/"+item.ID, item, nil); err != nil {
				return err
			}
		}
	}
}

func (ui *UI) providers(reader *bufio.Reader) error {
	for {
		var response providersResponse
		if err := ui.request(http.MethodGet, "/api/providers", nil, &response); err != nil {
			return err
		}
		items := response.Connections
		fmt.Fprintln(ui.Out, "\nProviders")
		for i, item := range items {
			fmt.Fprintf(ui.Out, "%d. %s (%s) [%s]\n", i+1, item.ID, item.BaseURL, map[bool]string{true: "enabled", false: "disabled"}[item.Enabled])
		}
		if !ui.huhMode() {
			fmt.Fprintln(ui.Out, "a. Add API provider  e. Edit  d. Delete  t. Test  b. Back")
		}
		var line string
		var err error
		if ui.huhMode() {
			choice, err := ui.huhChoice("Provider action", []string{"Add provider", "Edit provider", "Delete provider", "Test provider", "Back"})
			if err != nil {
				return err
			}
			line = map[string]string{"Add provider": "a", "Edit provider": "e", "Delete provider": "d", "Test provider": "t", "Back": "b"}[choice]
		} else {
			fmt.Fprint(ui.Out, "Select action: ")
			value, err := reader.ReadString('\n')
			if err != nil && len(value) == 0 {
				return err
			}
			line = value
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "b", "0":
			return nil
		case "a":
			if err := ui.promptProvider(&provider{Enabled: true, APIType: "openai"}, false); err != nil {
				return err
			}
		case "d", "e", "t":
			if len(items) == 0 {
				fmt.Fprintln(ui.Out, "No providers found.")
				continue
			}
			var index int
			if ui.huhMode() {
				labels := make([]string, 0, len(items))
				for _, item := range items {
					labels = append(labels, item.ID+" ("+item.BaseURL+")")
				}
				index, err = ui.huhNumber("Choose provider", labels)
			} else {
				fmt.Fprint(ui.Out, "Provider number: ")
				number, err := reader.ReadString('\n')
				if err != nil {
					return err
				}
				index, err = strconv.Atoi(strings.TrimSpace(number))
			}
			if err != nil || index < 1 || index > len(items) {
				fmt.Fprintln(ui.Out, "Invalid provider selection")
				continue
			}
			item := items[index-1]
			if strings.ToLower(strings.TrimSpace(line)) == "d" {
				ok, confirmErr := ui.huhConfirm("Delete provider "+item.ID+"?", false)
				if confirmErr != nil {
					return confirmErr
				}
				if !ok {
					continue
				}
				if err := ui.request(http.MethodDelete, "/api/providers?id="+item.ID, nil, nil); err != nil {
					return err
				}
				continue
			}
			if strings.ToLower(strings.TrimSpace(line)) == "e" {
				var current provider
				if err := ui.request(http.MethodGet, "/api/providers/"+item.ID, nil, &current); err != nil {
					return err
				}
				if ui.huhMode() {
					return ui.promptProvider(&current, true)
				}
				fmt.Fprintf(ui.Out, "Base URL (Enter keeps %s): ", current.BaseURL)
				value, err := reader.ReadString('\n')
				if err != nil {
					return err
				}
				if strings.TrimSpace(value) != "" {
					current.BaseURL = strings.TrimSpace(value)
				}
				fmt.Fprintf(ui.Out, "API key (Enter keeps current): ")
				value, err = reader.ReadString('\n')
				if err != nil {
					return err
				}
				if strings.TrimSpace(value) != "" {
					current.APIKey = strings.TrimSpace(value)
				}
				fmt.Fprintf(ui.Out, "Enabled [y/N]: ")
				value, err = reader.ReadString('\n')
				if err != nil {
					return err
				}
				current.Enabled = strings.EqualFold(strings.TrimSpace(value), "y")
				if err := ui.request(http.MethodPut, "/api/providers/"+item.ID, current, nil); err != nil {
					return err
				}
				continue
			}
			var result any
			if err := ui.request(http.MethodPost, "/api/providers/"+item.ID+"/test", nil, &result); err != nil {
				return err
			}
			fmt.Fprintln(ui.Out, "Test result:", result)
		default:
			fmt.Fprintln(ui.Out, "Invalid selection")
		}
	}
}

func (ui *UI) selectAuthMode(reader *bufio.Reader) (string, error) {
	if ui.huhMode() {
		choice, err := ui.huhChoice("Authentication", []string{"OAuth", "API key"})
		if err != nil {
			return "", err
		}
		if choice == "OAuth" {
			return "oauth", nil
		}
		return "apikey", nil
	}
	fmt.Fprintln(ui.Out, "\nSelect authentication")
	fmt.Fprintln(ui.Out, "1. OAuth")
	fmt.Fprintln(ui.Out, "2. API key")
	fmt.Fprint(ui.Out, "Authentication: ")
	choice, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "1", "o", "oauth":
		return "oauth", nil
	case "2", "a", "api", "api-key", "apikey":
		return "apikey", nil
	default:
		return "", fmt.Errorf("invalid authentication selection")
	}
}

func (ui *UI) selectProvider(reader *bufio.Reader, item *provider) error {
	ids := make([]string, 0, len(providers.Registry))
	for id := range providers.Registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if ui.huhMode() {
		options := append([]string{}, ids...)
		options = append(options, "Custom provider", "Back")
		choice, err := ui.huhChoice("Provider", options)
		if err != nil || choice == "Back" {
			return err
		}
		if choice == "Custom provider" {
			value, inputErr := ui.huhValue("Provider ID", "", false)
			if inputErr != nil {
				return inputErr
			}
			item.ID = strings.TrimSpace(value)
			return nil
		}
		descriptor := providers.Registry[choice]
		item.ID = descriptor.ID
		item.Name = descriptor.ID
		item.BaseURL = descriptor.BaseURL
		item.APIType = descriptor.Format
		return nil
	}
	fmt.Fprintln(ui.Out, "\nSelect provider (0. Custom provider)")
	for index, id := range ids {
		fmt.Fprintf(ui.Out, "%d. %s\n", index+1, id)
	}
	fmt.Fprint(ui.Out, "Provider: ")
	choice, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	choice = strings.TrimSpace(choice)
	index, err := strconv.Atoi(choice)
	if err == nil && index > 0 && index <= len(ids) {
		descriptor := providers.Registry[ids[index-1]]
		item.ID = descriptor.ID
		item.Name = descriptor.ID
		item.BaseURL = descriptor.BaseURL
		item.APIType = descriptor.Format
		return nil
	}
	if choice == "0" {
		fmt.Fprint(ui.Out, "Provider ID: ")
		value, readErr := reader.ReadString('\n')
		if readErr != nil {
			return readErr
		}
		item.ID = strings.TrimSpace(value)
		return nil
	}
	return fmt.Errorf("invalid provider selection")
}

func (ui *UI) apiKeys(reader *bufio.Reader) error {
	for {
		var payload struct {
			Keys []apiKey `json:"keys"`
		}
		if err := ui.request(http.MethodGet, "/api/keys", nil, &payload); err != nil {
			return err
		}
		fmt.Fprintln(ui.Out, "\nAPI Keys")
		for i, key := range payload.Keys {
			fmt.Fprintf(ui.Out, "%d. %s [%s] %s\n", i+1, key.Name, map[bool]string{true: "active", false: "inactive"}[key.IsActive], maskSecret(key.Key))
		}
		var line string
		var err error
		if ui.huhMode() {
			choice, choiceErr := ui.huhChoice("API key action", []string{"Create", "View full", "Copy", "Delete", "Toggle", "Back"})
			if choiceErr != nil {
				return choiceErr
			}
			line = map[string]string{"Create": "a", "View full": "v", "Copy": "c", "Delete": "d", "Toggle": "t", "Back": "b"}[choice]
		} else {
			fmt.Fprintln(ui.Out, "a. Create  v. View full  c. Copy  d. Delete  t. Toggle  b. Back")
			fmt.Fprint(ui.Out, "Select action: ")
			line, err = reader.ReadString('\n')
			if err != nil && len(line) == 0 {
				return err
			}
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "b", "0":
			return nil
		case "a":
			_, err := ui.promptAPIKey()
			if err != nil {
				return err
			}
			fmt.Fprintln(ui.Out, "API key created. Use View full to show it.")
		case "c", "d", "t", "v":
			if len(payload.Keys) == 0 {
				fmt.Fprintln(ui.Out, "No API keys found.")
				continue
			}
			var index int
			if ui.huhMode() {
				labels := make([]string, 0, len(payload.Keys))
				for _, key := range payload.Keys {
					labels = append(labels, key.Name)
				}
				index, err = ui.huhNumber("Choose API key", labels)
			} else {
				fmt.Fprint(ui.Out, "Key number: ")
				number, readErr := reader.ReadString('\n')
				if readErr != nil {
					return readErr
				}
				index, err = strconv.Atoi(strings.TrimSpace(number))
			}
			if err != nil {
				return err
			}
			if err != nil || index < 1 || index > len(payload.Keys) {
				fmt.Fprintln(ui.Out, "Invalid key number")
				continue
			}
			key := payload.Keys[index-1]
			if strings.ToLower(strings.TrimSpace(line)) == "v" {
				ok, confirmErr := ui.huhConfirm("Reveal this API key in terminal output?", false)
				if confirmErr != nil {
					return confirmErr
				}
				if !ok {
					continue
				}
				var detail struct {
					Key apiKey `json:"key"`
				}
				if err := ui.request(http.MethodGet, "/api/keys/"+key.ID, nil, &detail); err != nil {
					return err
				}
				fmt.Fprintf(ui.Out, "Name: %s\nID: %s\nKey: %s\n", detail.Key.Name, detail.Key.ID, detail.Key.Key)
				continue
			}
			if strings.ToLower(strings.TrimSpace(line)) == "c" {
				if err := copyClipboard(key.Key); err != nil {
					fmt.Fprintln(ui.Out, "Clipboard unavailable:", err)
				} else {
					fmt.Fprintln(ui.Out, "Key copied to clipboard")
				}
				continue
			}
			method, path, body := http.MethodDelete, "/api/keys/"+key.ID, any(nil)
			if strings.ToLower(strings.TrimSpace(line)) == "d" && ui.huhMode() {
				ok, confirmErr := ui.huhConfirm("Delete this API key?", false)
				if confirmErr != nil {
					return confirmErr
				}
				if !ok {
					continue
				}
			}
			if strings.ToLower(strings.TrimSpace(line)) == "t" {
				method, body = http.MethodPut, map[string]bool{"isActive": !key.IsActive}
			}
			if err := ui.request(method, path, body, nil); err != nil {
				return err
			}
		default:
			fmt.Fprintln(ui.Out, "Invalid selection")
		}
	}
}

func copyClipboard(value string) error {
	for _, command := range []string{"pbcopy", "wl-copy", "xclip", "xsel"} {
		if _, err := exec.LookPath(command); err != nil {
			continue
		}
		arguments := []string{}
		if command == "xclip" {
			arguments = []string{"-selection", "clipboard"}
		}
		if command == "xsel" {
			arguments = []string{"--clipboard", "--input"}
		}
		process := exec.Command(command, arguments...)
		process.Stdin = strings.NewReader(value)
		if err := process.Run(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no clipboard command succeeded")
}

func (ui *UI) request(method, path string, body any, result any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(data))
	}
	request, err := http.NewRequest(method, ui.BaseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("X-G9Router-Local-CLI", "1")
	response, err := ui.Client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return err
	}
	if response.StatusCode >= 400 {
		return fmt.Errorf("HTTP %s", response.Status)
	}
	if result != nil && len(data) > 0 && json.Unmarshal(data, result) != nil {
		return fmt.Errorf("invalid JSON response")
	}
	return nil
}

func PortURL(host string, port int) string {
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 20128
	}
	return "http://" + host + ":" + strconv.Itoa(port)
}

func IsTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
