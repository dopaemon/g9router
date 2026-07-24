package translator

import (
	"strings"
	"testing"
)

func TestKiroEventToOpenAISSE(t *testing.T) {
	state := &KiroStreamState{}
	text := KiroEventToOpenAISSE("assistantResponseEvent", []byte(`{"content":"hello"}`), state)
	if len(text) != 1 || !strings.Contains(text[0], `"content":"hello"`) {
		t.Fatal(text)
	}
	finish := KiroEventToOpenAISSE("messageStopEvent", []byte(`{}`), state)
	if len(finish) != 1 || !strings.Contains(finish[0], `"finish_reason":"stop"`) {
		t.Fatal(finish)
	}
}
