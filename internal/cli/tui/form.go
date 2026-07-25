package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type formModel struct {
	title   string
	path    string
	baseURL string
	client  *http.Client
	fields  []textinput.Model
	index   int
	busy    bool
	err     error
}

type formSavedMsg struct{ err error }

func newForm(title, baseURL, path string, client *http.Client, labels ...string) formModel {
	fields := make([]textinput.Model, len(labels))
	for index, label := range labels {
		field := textinput.New()
		field.Prompt = label + ": "
		field.CharLimit = 512
		field.Width = 52
		fields[index] = field
	}
	if len(fields) > 0 {
		fields[0].Focus()
	}
	return formModel{title: title, baseURL: baseURL, path: path, client: client, fields: fields}
}

func (form formModel) update(message tea.Msg) (formModel, tea.Cmd) {
	if form.busy {
		return form, nil
	}
	if key, ok := message.(tea.KeyMsg); ok {
		switch strings.ToLower(key.String()) {
		case "esc", "b":
			form.err = nil
			return form, nil
		case "tab", "down":
			form.fields[form.index].Blur()
			form.index = (form.index + 1) % len(form.fields)
			form.fields[form.index].Focus()
		case "shift+tab", "up":
			form.fields[form.index].Blur()
			form.index = (form.index - 1 + len(form.fields)) % len(form.fields)
			form.fields[form.index].Focus()
		case "enter":
			payload := make(map[string]string, len(form.fields))
			for _, field := range form.fields {
				if strings.TrimSpace(field.Value()) == "" {
					form.err = fmt.Errorf("%s is required", strings.TrimSpace(field.Prompt))
					return form, nil
				}
				label := strings.TrimSuffix(strings.TrimSpace(field.Prompt), ":")
				key := strings.ToLower(label)
				switch label {
				case "Base URL":
					key = "baseURL"
				case "API key":
					key = "apiKey"
				}
				payload[key] = strings.TrimSpace(field.Value())
			}
			form.busy = true
			return form, saveForm(form.baseURL, form.path, form.client, payload)
		}
	}
	var command tea.Cmd
	form.fields[form.index], command = form.fields[form.index].Update(message)
	return form, command
}

func (form formModel) view() string {
	content := styles.Title.Render(form.title) + "\n\n"
	for index, field := range form.fields {
		content += field.View() + "\n"
		if index == form.index {
			content += styles.Muted.Render("  active field") + "\n"
		}
	}
	if form.busy {
		content += "\n" + styles.Warning.Render("Saving…")
	} else if form.err != nil {
		content += "\n" + styles.Error.Render("✗ "+form.err.Error())
	}
	content += "\n\n" + styles.Muted.Render("tab/↑↓ next field  enter save  esc cancel")
	return content
}

func saveForm(baseURL, path string, client *http.Client, payload map[string]string) tea.Cmd {
	return func() tea.Msg {
		data, err := json.Marshal(payload)
		if err != nil {
			return formSavedMsg{err: err}
		}
		request, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(data))
		if err != nil {
			return formSavedMsg{err: err}
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return formSavedMsg{err: err}
		}
		defer response.Body.Close()
		if response.StatusCode >= 400 {
			return formSavedMsg{err: fmt.Errorf("HTTP %s", response.Status)}
		}
		return formSavedMsg{}
	}
}
