package tui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestRunCLIToolsBack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cli-tools/all-statuses" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"claude":{"installed":true,"configured":false}}`)
	}))
	defer server.Close()

	var output bytes.Buffer
	input := strings.NewReader("4\nb\n0\n")
	if err := Run(server.URL, input, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "claude") {
		t.Fatalf("output does not contain tool status: %s", output.String())
	}
}

func TestQuickSetupOpenCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/keys":
			_, _ = fmt.Fprint(w, `{"keys":[{"key":"sk-test"}]}`)
		case "/api/cli-tools/opencode-settings":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["apiKey"] != "sk-test" || body["activeModel"] != "cc/test" {
				t.Fatalf("payload = %#v", body)
			}
			models, ok := body["models"].([]any)
			if !ok || len(models) != 1 || models[0] != "cc/test" {
				t.Fatalf("models = %#v", body["models"])
			}
			_, _ = fmt.Fprint(w, `{"success":true}`)
		default:
			_, _ = fmt.Fprint(w, `{}`)
		}
	}))
	defer server.Close()

	ui := &UI{BaseURL: server.URL, In: strings.NewReader(""), Out: &bytes.Buffer{}, Client: server.Client()}
	if err := ui.quickSetup(bufio.NewReader(strings.NewReader("opencode\ncc/test\n"))); err != nil {
		t.Fatal(err)
	}
}

func TestSelectClaudeModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cli-tools/claude-settings" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body map[string]map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["env"]["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "cc/opus-test" {
			t.Fatalf("payload = %#v", body)
		}
		_, _ = fmt.Fprint(w, `{"success":true}`)
	}))
	defer server.Close()

	ui := &UI{BaseURL: server.URL, Out: &bytes.Buffer{}, Client: server.Client()}
	if err := ui.selectClaudeModel(bufio.NewReader(strings.NewReader("opus\ncc/opus-test\n"))); err != nil {
		t.Fatal(err)
	}
}

func TestProvidersReadsConnectionsEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/providers" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"connections":[{"id":"openai","baseURL":"https://example.com","enabled":true}]}`)
	}))
	defer server.Close()

	var output strings.Builder
	ui := &UI{BaseURL: server.URL, Out: &output, Client: server.Client()}
	if err := ui.providers(bufio.NewReader(strings.NewReader("b\n"))); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "openai (https://example.com) [enabled]") {
		t.Fatalf("output=%s", output.String())
	}
}

func TestSelectProviderUsesRegistryDefaults(t *testing.T) {
	ui := &UI{Out: &bytes.Buffer{}}
	item := provider{}
	if err := ui.selectProvider(bufio.NewReader(strings.NewReader("1\n")), &item); err != nil {
		t.Fatal(err)
	}
	if item.ID == "" || item.Name == "" || item.APIType == "" {
		t.Fatalf("provider defaults = %#v", item)
	}
}

func TestSelectAuthModeAcceptsOAuthAndAPIKey(t *testing.T) {
	ui := &UI{Out: &bytes.Buffer{}}
	mode, err := ui.selectAuthMode(bufio.NewReader(strings.NewReader("1\n")))
	if err != nil || mode != "oauth" {
		t.Fatalf("oauth mode=%q err=%v", mode, err)
	}
	mode, err = ui.selectAuthMode(bufio.NewReader(strings.NewReader("2\n")))
	if err != nil || mode != "apikey" {
		t.Fatalf("api key mode=%q err=%v", mode, err)
	}
}

func TestGradientAndTeaViewRenderAtNarrowWidth(t *testing.T) {
	model := newTeaModel("http://127.0.0.1:20128")
	if !strings.Contains(model.View(), "Providers") {
		t.Fatalf("root menu missing providers: %s", model.View())
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	view := updated.(teaModel).View()
	if !strings.Contains(view, "9ROUTER") || !strings.Contains(view, "Providers") {
		t.Fatalf("view=%q", view)
	}
	if gradient("") != "" {
		t.Fatal("empty gradient should stay empty")
	}
}

func TestLoadScreenFormatsJSONAndHTTPFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			_, _ = fmt.Fprint(w, `{"items":[{"name":"rose"}]}`)
			return
		}
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprint(w, `{"error":"upstream unavailable"}`)
	}))
	defer server.Close()

	loaded := loadScreen(server.URL, server.Client(), "/ok")()
	message, ok := loaded.(screenLoadedMsg)
	if !ok || message.err != nil || !strings.Contains(message.content, "rose") {
		t.Fatalf("loaded = %#v", loaded)
	}

	failed := loadScreen(server.URL, server.Client(), "/fail")().(screenLoadedMsg)
	if failed.err == nil || !strings.Contains(failed.err.Error(), "502") {
		t.Fatalf("failure = %#v", failed.err)
	}
}

func TestProvidersScreenRendersConnectionTable(t *testing.T) {
	model := newScreenModel("http://example.test", &bytes.Buffer{}, http.DefaultClient)
	model.current = &screens[0]
	model.loading = false
	model.width = 100
	model.height = 30
	model.viewport = viewport.New(92, 19)
	updated, _ := model.Update(screenLoadedMsg{content: `{"connections":[{"id":"openai","baseURL":"https://api.openai.com","enabled":true}]}`})
	model = updated.(screenModel)
	view := model.View()
	if !strings.Contains(view, "openai") || !strings.Contains(view, "enabled") {
		t.Fatalf("provider view missing table data: %s", view)
	}
	if strings.Contains(view, `"connections"`) {
		t.Fatalf("provider view leaked raw JSON: %s", view)
	}
}

func TestProvidersScreenSupportsSelection(t *testing.T) {
	model := newScreenModel("http://example.test", &bytes.Buffer{}, http.DefaultClient)
	model.current = &screens[0]
	model.viewport = viewport.New(92, 19)
	message := screenLoadedMsg{content: `{"connections":[{"id":"openai","baseURL":"https://openai.test","enabled":true},{"id":"anthropic","baseURL":"https://anthropic.test","enabled":false}]}`}
	updated, _ := model.Update(message)
	model = updated.(screenModel)
	if model.selected != 0 || !strings.Contains(model.View(), "▸") {
		t.Fatalf("initial selection missing: %s", model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(screenModel)
	if model.selected != 1 || !strings.Contains(model.View(), "anthropic") {
		t.Fatalf("selection did not move: %s", model.View())
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(screenModel)
	if !strings.Contains(model.notice, "anthropic") {
		t.Fatalf("enter did not select provider: %q", model.notice)
	}
}

func TestNonResourceScreensUseReadableViews(t *testing.T) {
	settings := formatScreenContent("Settings", `{"rtkEnabled":true,"headroom":false}`)
	if !strings.Contains(settings, "SETTING") || strings.Contains(settings, `"rtkEnabled"`) {
		t.Fatalf("settings view=%s", settings)
	}
	oauth := formatScreenContent("OAuth", `{"cred-1":{"provider":"claude","active":true}}`)
	if !strings.Contains(oauth, "PROVIDER") || strings.Contains(oauth, `"cred-1"`) {
		t.Fatalf("oauth view=%s", oauth)
	}
}

func TestResourceActionsUseSelectedItemAndHTTPContract(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.RequestURI()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	model := newScreenModel(server.URL, &bytes.Buffer{}, server.Client())
	model.current = &screens[0]
	model.items = []resourceItem{{id: "openai", label: "openai", status: "enabled"}}
	action := model.actionFor("d")
	if action == nil || action.path != "/api/providers?id=openai" {
		t.Fatalf("action=%#v", action)
	}
	message := runResourceAction(server.URL, server.Client(), *action)()
	if message.(actionDoneMsg).err != nil || method != http.MethodDelete || path != "/api/providers?id=openai" {
		t.Fatalf("request=%s %s result=%#v", method, path, message)
	}
}

func TestScreenModelLoadsAfterRootSelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"connections":[]}`)
	}))
	defer server.Close()

	model := newScreenModel(server.URL, &bytes.Buffer{}, server.Client())
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = updated.(screenModel)
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(screenModel)
	if model.current == nil || command == nil {
		t.Fatalf("selection did not start load: current=%#v command=%v", model.current, command)
	}
	updated, _ = model.Update(command())
	model = updated.(screenModel)
	if model.loading || model.err != nil || !strings.Contains(model.View(), "No providers configured") {
		t.Fatalf("load state: loading=%v err=%v view=%s", model.loading, model.err, model.View())
	}
}

func TestScreenModelUsesMinimumViewportWhenTerminalSizeIsUnknown(t *testing.T) {
	model := newScreenModel("http://example.test", &bytes.Buffer{}, http.DefaultClient)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 0, Height: 0})
	model = updated.(screenModel)
	if model.viewport.Width != 40 || model.viewport.Height != 8 {
		t.Fatalf("viewport=%dx%d", model.viewport.Width, model.viewport.Height)
	}
}
