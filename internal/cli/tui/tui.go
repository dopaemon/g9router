package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

type UI struct {
	BaseURL string
	In      io.Reader
	Out     io.Writer
	Client  *http.Client
}

func Run(baseURL string, in io.Reader, out io.Writer) error {
	ui := &UI{BaseURL: strings.TrimRight(baseURL, "/"), In: in, Out: out, Client: http.DefaultClient}
	reader := bufio.NewReader(in)
	for {
		fmt.Fprintln(out, "\n9Router Terminal UI")
		fmt.Fprintln(out, "1. Providers")
		fmt.Fprintln(out, "2. API Keys")
		fmt.Fprintln(out, "3. Combos")
		fmt.Fprintln(out, "4. CLI Tools")
		fmt.Fprintln(out, "5. Settings")
		fmt.Fprintln(out, "0. Exit")
		fmt.Fprint(out, "Select option: ")
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return err
		}
		switch strings.TrimSpace(line) {
		case "0", "q", "Q":
			return nil
		case "1":
			err = ui.providers(reader)
		case "2":
			err = ui.apiKeys(reader)
		case "3":
			err = ui.combos(reader)
		case "4":
			err = ui.cliTools(reader)
		case "5":
			err = ui.settings(reader)
		default:
			fmt.Fprintln(out, "Invalid selection")
		}
		if err != nil {
			fmt.Fprintln(out, "Error:", err)
		}
	}
}

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
		pretty, _ := json.MarshalIndent(value, "", "  ")
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
	Enabled bool   `json:"enabled"`
}

type combo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Models []any  `json:"models"`
	Kind   string `json:"kind,omitempty"`
}

func (ui *UI) cliTools(reader *bufio.Reader) error {
	paths := map[string]string{"claude": "/api/cli-tools/claude-settings", "codex": "/api/cli-tools/codex-settings", "opencode": "/api/cli-tools/opencode-settings", "copilot": "/api/cli-tools/copilot-settings", "droid": "/api/cli-tools/droid-settings", "cline": "/api/cli-tools/cline-settings", "kilo": "/api/cli-tools/kilo-settings", "openclaw": "/api/cli-tools/openclaw-settings", "deepseek-tui": "/api/cli-tools/deepseek-tui-settings", "grok-build": "/api/cli-tools/grok-build-settings", "hermes": "/api/cli-tools/hermes-settings", "jcode": "/api/cli-tools/jcode-settings", "cowork": "/api/cli-tools/cowork-settings"}
	for {
		var statuses map[string]any
		if err := ui.request(http.MethodGet, "/api/cli-tools/all-statuses", nil, &statuses); err != nil {
			return err
		}
		fmt.Fprintln(ui.Out, "\nCLI Tools")
		keys := make([]string, 0, len(statuses))
		for name := range statuses {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		for i, name := range keys {
			fmt.Fprintf(ui.Out, "%d. %s: %v\n", i+1, name, statuses[name])
		}
		fmt.Fprintln(ui.Out, "s. Show settings  a. Apply JSON  r. Reset  b. Back")
		fmt.Fprint(ui.Out, "Select action: ")
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return err
		}
		action := strings.ToLower(strings.TrimSpace(line))
		if action == "b" || action == "0" {
			return nil
		}
		if action != "s" && action != "a" && action != "r" {
			fmt.Fprintln(ui.Out, "Invalid selection")
			continue
		}
		fmt.Fprint(ui.Out, "Tool number: ")
		line, err = reader.ReadString('\n')
		if err != nil {
			return err
		}
		index, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || index < 1 || index > len(keys) {
			fmt.Fprintln(ui.Out, "Invalid tool number")
			continue
		}
		path := paths[keys[index-1]]
		if path == "" {
			fmt.Fprintln(ui.Out, "Tool settings are not available")
			continue
		}
		if action == "s" {
			if err := ui.showJSON(reader, path); err != nil {
				return err
			}
			continue
		}
		if action == "r" {
			if err := ui.request(http.MethodDelete, path, nil, nil); err != nil {
				return err
			}
			continue
		}
		fmt.Fprint(ui.Out, "JSON body: ")
		body, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		var payload any
		if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &payload); err != nil {
			fmt.Fprintln(ui.Out, "Invalid JSON")
			continue
		}
		if err := ui.request(http.MethodPost, path, payload, nil); err != nil {
			return err
		}
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
		fmt.Fprintf(ui.Out, "\nSettings\nRTK: %v  Headroom: %v  Tunnel: %v\n", values["rtkEnabled"] != false, values["headroomEnabled"] == true, tunnel["enabled"] == true)
		fmt.Fprintln(ui.Out, "1. Toggle RTK  2. Toggle Headroom  3. Tunnel ON  4. Tunnel OFF  5. Reset auth mode  b. Back")
		fmt.Fprint(ui.Out, "Select action: ")
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return err
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
			if err := ui.request(http.MethodPost, "/api/tunnel/disable", nil, nil); err != nil {
				return err
			}
		case "5":
			if err := ui.request(http.MethodPut, "/api/settings", map[string]string{"authMode": "password"}, nil); err != nil {
				return err
			}
		default:
			fmt.Fprintln(ui.Out, "Invalid selection")
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
		fmt.Fprintln(ui.Out, "\nCombos")
		for i, item := range payload.Combos {
			fmt.Fprintf(ui.Out, "%d. %s: %v\n", i+1, item.Name, item.Models)
		}
		fmt.Fprintln(ui.Out, "a. Create  e. Edit  d. Delete  b. Back")
		fmt.Fprint(ui.Out, "Select action: ")
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return err
		}
		action := strings.ToLower(strings.TrimSpace(line))
		if action == "b" || action == "0" {
			return nil
		}
		if action == "a" {
			var item combo
			fmt.Fprint(ui.Out, "Name: ")
			value, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			item.Name = strings.TrimSpace(value)
			fmt.Fprint(ui.Out, "Models (comma-separated): ")
			value, err = reader.ReadString('\n')
			if err != nil {
				return err
			}
			for _, model := range strings.Split(strings.TrimSpace(value), ",") {
				if strings.TrimSpace(model) != "" {
					item.Models = append(item.Models, strings.TrimSpace(model))
				}
			}
			if err := ui.request(http.MethodPost, "/api/combos", item, nil); err != nil {
				return err
			}
			continue
		}
		if action != "e" && action != "d" || len(payload.Combos) == 0 {
			fmt.Fprintln(ui.Out, "Invalid selection")
			continue
		}
		fmt.Fprint(ui.Out, "Combo number: ")
		value, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		index, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || index < 1 || index > len(payload.Combos) {
			fmt.Fprintln(ui.Out, "Invalid combo number")
			continue
		}
		item := payload.Combos[index-1]
		if action == "d" {
			if err := ui.request(http.MethodDelete, "/api/combos/"+item.ID, nil, nil); err != nil {
				return err
			}
			continue
		}
		fmt.Fprintf(ui.Out, "New name (Enter keeps %s): ", item.Name)
		value, err = reader.ReadString('\n')
		if err != nil {
			return err
		}
		if strings.TrimSpace(value) != "" {
			item.Name = strings.TrimSpace(value)
		}
		if err := ui.request(http.MethodPut, "/api/combos/"+item.ID, item, nil); err != nil {
			return err
		}
	}
}

func (ui *UI) providers(reader *bufio.Reader) error {
	for {
		var items []provider
		if err := ui.request(http.MethodGet, "/api/providers", nil, &items); err != nil {
			return err
		}
		fmt.Fprintln(ui.Out, "\nProviders")
		for i, item := range items {
			fmt.Fprintf(ui.Out, "%d. %s (%s) [%s]\n", i+1, item.ID, item.BaseURL, map[bool]string{true: "enabled", false: "disabled"}[item.Enabled])
		}
		fmt.Fprintln(ui.Out, "a. Add API provider  e. Edit  d. Delete  t. Test  b. Back")
		fmt.Fprint(ui.Out, "Select action: ")
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "b", "0":
			return nil
		case "a":
			item := provider{Enabled: true, APIType: "openai"}
			for _, field := range []struct {
				name   string
				target *string
			}{{"Provider ID", &item.ID}, {"Name", &item.Name}, {"Base URL", &item.BaseURL}, {"API key", &item.APIKey}} {
				fmt.Fprintf(ui.Out, "%s: ", field.name)
				value, err := reader.ReadString('\n')
				if err != nil {
					return err
				}
				*field.target = strings.TrimSpace(value)
			}
			if err := ui.request(http.MethodPost, "/api/providers", item, nil); err != nil {
				return err
			}
		case "d", "e", "t":
			if len(items) == 0 {
				fmt.Fprintln(ui.Out, "No providers found.")
				continue
			}
			fmt.Fprint(ui.Out, "Provider number: ")
			number, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			index, err := strconv.Atoi(strings.TrimSpace(number))
			if err != nil || index < 1 || index > len(items) {
				fmt.Fprintln(ui.Out, "Invalid provider number")
				continue
			}
			item := items[index-1]
			if strings.ToLower(strings.TrimSpace(line)) == "d" {
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
			fmt.Fprintf(ui.Out, "%d. %s [%s] %s\n", i+1, key.Name, map[bool]string{true: "active", false: "inactive"}[key.IsActive], key.Key)
		}
		fmt.Fprintln(ui.Out, "a. Create  d. Delete  t. Toggle  b. Back")
		fmt.Fprint(ui.Out, "Select action: ")
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "b", "0":
			return nil
		case "a":
			fmt.Fprint(ui.Out, "Key name: ")
			name, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			var created apiKey
			if err := ui.request(http.MethodPost, "/api/keys", map[string]string{"name": strings.TrimSpace(name)}, &created); err != nil {
				return err
			}
			fmt.Fprintf(ui.Out, "Created key: %s\nSave it now; it is not shown again.\n", created.Key)
		case "d", "t":
			if len(payload.Keys) == 0 {
				fmt.Fprintln(ui.Out, "No API keys found.")
				continue
			}
			fmt.Fprint(ui.Out, "Key number: ")
			number, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			index, err := strconv.Atoi(strings.TrimSpace(number))
			if err != nil || index < 1 || index > len(payload.Keys) {
				fmt.Fprintln(ui.Out, "Invalid key number")
				continue
			}
			key := payload.Keys[index-1]
			method, path, body := http.MethodDelete, "/api/keys/"+key.ID, any(nil)
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
		return fmt.Errorf("HTTP %s: %s", response.Status, strings.TrimSpace(string(data)))
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
