package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyKiroSSEJSON(t *testing.T) {
	recorder := httptest.NewRecorder()
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"lo\",\"reasoning_content\":\"why\"}}]}\n\ndata: [DONE]\n\n"
	if !new(Server).proxyKiroSSEJSON(recorder, strings.NewReader(input), "claude-sonnet") {
		t.Fatal("proxyKiroSSEJSON returned false")
	}
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), `"content":"hello"`) || !strings.Contains(recorder.Body.String(), `"reasoning_content":"why"`) {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
}
