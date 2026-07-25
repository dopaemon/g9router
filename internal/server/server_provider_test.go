package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	providernodes "g9router/internal/providernodes"
	"g9router/internal/providers"
)

func TestProviderProxyBaseURLUsesTokenPlanRegion(t *testing.T) {
	provider := providers.Provider{ID: "xiaomi-tokenplan", BaseURL: "https://token-plan-sgp.xiaomimimo.com/v1/chat/completions", ProviderSpecificData: map[string]any{"region": "ams"}}
	if got := providerProxyBaseURL(provider); got != "https://token-plan-ams.xiaomimimo.com/v1" {
		t.Fatalf("base URL = %q", got)
	}
}

func TestValidateAzureRequiresEndpoint(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/providers/validate", strings.NewReader(`{"provider":"azure","apiKey":"secret","providerSpecificData":{}}`))
	request.Header.Set("Content-Type", "application/json")
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Missing Azure endpoint") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestValidateCloudflareRequiresAccount(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/providers/validate", strings.NewReader(`{"provider":"cloudflare-ai","apiKey":"secret","providerSpecificData":{}}`))
	request.Header.Set("Content-Type", "application/json")
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Missing Account ID") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestValidateOpenAICompatibleNode(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("path=%s authorization=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	nodesPath := t.TempDir() + "/nodes.json"
	nodes, _ := json.Marshal([]map[string]string{{"id": "node-1", "type": "openai-compatible", "baseUrl": upstream.URL}})
	if err := os.WriteFile(nodesPath, nodes, 0600); err != nil {
		t.Fatal(err)
	}
	app := New(Options{ProviderNodesPath: nodesPath, ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/providers/validate", strings.NewReader(`{"provider":"node-1","apiKey":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"valid":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestValidateOllamaLocalAllowsEmptyKey(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/providers/validate", strings.NewReader(`{"provider":"ollama-local","providerSpecificData":{"baseUrl":"`+upstream.URL+`"}}`))
	request.Header.Set("Content-Type", "application/json")
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"valid":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestValidateXiaomiTokenPlanUsesSelectedRegion(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	app.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "token-plan-cn.xiaomimimo.com" || request.URL.Path != "/v1/models" {
			t.Fatalf("url=%s", request.URL.String())
		}
		return &http.Response{StatusCode: http.StatusForbidden, Body: http.NoBody, Header: make(http.Header)}, nil
	})}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/providers/validate", strings.NewReader(`{"provider":"xiaomi-tokenplan","apiKey":"secret","providerSpecificData":{"region":"cn"}}`))
	request.Header.Set("Content-Type", "application/json")
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"valid":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestValidateXAIAcceptsForbiddenButRejectsBadRequest(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusBadRequest} {
		app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
		app.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Host != "api.x.ai" || request.URL.Path != "/v1/models" {
				t.Fatalf("url=%s", request.URL.String())
			}
			return &http.Response{StatusCode: status, Body: http.NoBody, Header: make(http.Header)}, nil
		})}
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/providers/validate", strings.NewReader(`{"provider":"xai","apiKey":"secret"}`))
		request.Header.Set("Content-Type", "application/json")
		app.Handler().ServeHTTP(response, request)
		want := status == http.StatusForbidden
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"valid":true`) != want {
			t.Fatalf("status=%d response=%s want=%v", status, response.Body.String(), want)
		}
	}
}

func TestValidateOpenAIRequiresSuccessStatus(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusInternalServerError} {
		app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
		app.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Host != "api.openai.com" || request.URL.Path != "/v1/models" {
				t.Fatalf("url=%s", request.URL.String())
			}
			return &http.Response{StatusCode: status, Body: http.NoBody, Header: make(http.Header)}, nil
		})}
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/providers/validate", strings.NewReader(`{"provider":"openai","apiKey":"secret"}`))
		request.Header.Set("Content-Type", "application/json")
		app.Handler().ServeHTTP(response, request)
		want := status == http.StatusOK
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"valid":true`) != want {
			t.Fatalf("status=%d response=%s want=%v", status, response.Body.String(), want)
		}
	}
}

func TestValidateAnthropicRejectsUnauthorizedOnly(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	app.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "api.anthropic.com" || request.URL.Path != "/v1/messages" {
			t.Fatalf("url=%s", request.URL.String())
		}
		return &http.Response{StatusCode: http.StatusBadRequest, Body: http.NoBody, Header: make(http.Header)}, nil
	})}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/providers/validate", strings.NewReader(`{"provider":"anthropic","apiKey":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"valid":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestValidateGenericProviderFallsBackToChat(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	app.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/models":
			return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: make(http.Header)}, nil
		case "/chat/completions":
			return &http.Response{StatusCode: http.StatusBadRequest, Body: http.NoBody, Header: make(http.Header)}, nil
		default:
			t.Fatalf("path=%s", request.URL.Path)
			return nil, nil
		}
	})}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/providers/validate", strings.NewReader(`{"provider":"deepseek","apiKey":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"valid":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestValidateNoAuthProviderAllowsEmptyKey(t *testing.T) {
	for _, provider := range []string{"opencode", "searxng"} {
		app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/providers/validate", strings.NewReader(`{"provider":"`+provider+`"}`))
		request.Header.Set("Content-Type", "application/json")
		app.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"valid":true`) {
			t.Fatalf("provider=%s status=%d body=%s", provider, response.Code, response.Body.String())
		}
	}
}

func TestProviderBatchSupportsFreeAndCompatibleModes(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	for _, provider := range []providers.Provider{
		{ID: "opencode", BaseURL: "http://127.0.0.1:1", Enabled: true},
		{ID: "openai-compatible-demo", BaseURL: "http://127.0.0.1:1", Enabled: true},
	} {
		if err := app.store.Upsert(provider); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"free", "compatible"} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/providers/test-batch", strings.NewReader(`{"mode":"`+mode+`"}`))
		request.Header.Set("Content-Type", "application/json")
		app.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"mode":"`+mode+`"`) {
			t.Fatalf("mode=%s status=%d body=%s", mode, response.Code, response.Body.String())
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestProviderNodeUpdateAndDeleteSyncsConnections(t *testing.T) {
	app := New(Options{ProviderNodesPath: t.TempDir() + "/nodes.json", ProviderPath: t.TempDir() + "/providers.json"})
	node, err := app.providerNodes.Create(providernodes.Node{Type: "openai-compatible", Prefix: "node", APIType: "chat", BaseURL: "http://old", Name: "Old"})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.store.Upsert(providers.Provider{ID: node.ID, BaseURL: node.BaseURL, APIKey: "secret", Enabled: true, ProviderSpecificData: map[string]any{"prefix": node.Prefix, "baseUrl": node.BaseURL, "nodeName": node.Name}}); err != nil {
		t.Fatal(err)
	}

	update := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/provider-nodes/"+node.ID, strings.NewReader(`{"name":"New","prefix":"new","apiType":"responses","baseUrl":"http://new"}`))
	app.Handler().ServeHTTP(update, request)
	if update.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", update.Code, update.Body.String())
	}
	provider, ok := app.store.Find(node.ID)
	if !ok || provider.APIKey != "secret" || provider.ProviderSpecificData["prefix"] != "new" || provider.ProviderSpecificData["baseUrl"] != "http://new" || provider.ProviderSpecificData["nodeName"] != "New" || provider.ProviderSpecificData["apiType"] != "responses" {
		t.Fatalf("connection not synchronized: %#v", provider)
	}

	remove := httptest.NewRecorder()
	app.Handler().ServeHTTP(remove, httptest.NewRequest(http.MethodDelete, "/api/provider-nodes/"+node.ID, nil))
	if remove.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", remove.Code, remove.Body.String())
	}
	if _, ok := app.providerNodes.Get(node.ID); ok {
		t.Fatal("provider node still exists")
	}
	if _, ok := app.store.Find(node.ID); ok {
		t.Fatal("provider connection still exists")
	}
}
