package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranslatorAPI(t *testing.T) {
	app := New(Options{ProviderPath: t.TempDir() + "/providers.json", OAuthPath: t.TempDir() + "/oauth.json"})
	stepOne := httptest.NewRecorder()
	app.Handler().ServeHTTP(stepOne, httptest.NewRequest(http.MethodPost, "/api/translator/translate", strings.NewReader(`{"step":1,"body":{"model":"anthropic/claude-sonnet-4","messages":[]}}`)))
	if stepOne.Code != http.StatusOK || !strings.Contains(stepOne.Body.String(), `"provider":"anthropic"`) {
		t.Fatalf("step1 status=%d body=%s", stepOne.Code, stepOne.Body.String())
	}
	stepTwo := httptest.NewRecorder()
	app.Handler().ServeHTTP(stepTwo, httptest.NewRequest(http.MethodPost, "/api/translator/translate", strings.NewReader(`{"step":2,"body":{"model":"openai/gpt-5","input":"hello"}}`)))
	if stepTwo.Code != http.StatusOK || !strings.Contains(stepTwo.Body.String(), `"messages"`) {
		t.Fatalf("step2 status=%d body=%s", stepTwo.Code, stepTwo.Body.String())
	}
}
