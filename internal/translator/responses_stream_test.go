package translator

import "testing"

func TestResponsesEventToChatSSE(t *testing.T) {
	state := &ResponsesStreamState{}
	events := ResponsesEventToChatSSE([]byte(`{"type":"response.output_text.delta","response_id":"r","delta":"hi"}`), state)
	if len(events) != 2 {
		t.Fatal(events)
	}
	done := ResponsesEventToChatSSE([]byte(`{"type":"response.completed","response_id":"r"}`), state)
	if len(done) != 2 {
		t.Fatal(done)
	}
}
