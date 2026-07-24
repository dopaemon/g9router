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
			err = ui.showJSON(reader, "/api/keys")
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
