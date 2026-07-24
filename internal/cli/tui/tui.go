package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
			err = ui.showJSON(reader, "/api/providers")
		case "2":
			err = ui.apiKeys(reader)
		case "3":
			err = ui.showJSON(reader, "/api/combos")
		case "4":
			err = ui.showJSON(reader, "/api/cli-tools/all-statuses")
		case "5":
			err = ui.showJSON(reader, "/api/settings")
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
