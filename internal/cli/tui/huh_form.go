package tui

import (
	"net/http"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

type huhFormState struct {
	form       *huh.Form
	kind       string
	providerID string
	name       string
	baseURL    string
	apiKey     string
}

func newProviderHuhForm() *huhFormState {
	state := &huhFormState{kind: "provider"}
	state.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Provider ID").Description("Stable ID, e.g. openai or anthropic").Value(&state.providerID).Validate(huh.ValidateNotEmpty()),
			huh.NewInput().Title("Display name").Value(&state.name),
			huh.NewInput().Title("Base URL").Description("OpenAI-compatible upstream URL").Value(&state.baseURL).Validate(huh.ValidateNotEmpty()),
			huh.NewInput().Title("API key").EchoMode(huh.EchoModePassword).Value(&state.apiKey).Validate(huh.ValidateNotEmpty()),
		),
	).WithTheme(huh.ThemeCharm()).WithWidth(72).WithShowHelp(true)
	return state
}

func newAPIKeyHuhForm() *huhFormState {
	state := &huhFormState{kind: "key"}
	state.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Key name").Description("A recognizable name for this gateway key").Value(&state.name).Validate(huh.ValidateNotEmpty()),
		),
	).WithTheme(huh.ThemeCharm()).WithWidth(72).WithShowHelp(true)
	return state
}

func (state *huhFormState) payload() (string, string, any) {
	if state.kind == "provider" {
		return http.MethodPost, "/api/providers", map[string]any{
			"id": state.providerID, "name": state.name, "baseURL": state.baseURL,
			"apiKey": state.apiKey, "apiType": "openai", "enabled": true,
		}
	}
	return http.MethodPost, "/api/keys", map[string]any{"name": state.name}
}

func saveHuhForm(baseURL string, client *http.Client, method, path string, body any, _ string) tea.Cmd {
	return runResourceAction(baseURL, client, resourceAction{method: method, path: path, body: body, label: "save " + path})
}
