package mcp

import (
	"context"
	"io"
	"testing"
)

func TestBridgeRoundTrip(t *testing.T) {
	bridge := New()
	id, lines, err := bridge.Start(context.Background(), "cat")
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close(id)
	if err := bridge.Send(id, map[string]any{"jsonrpc": "2.0"}); err != nil {
		t.Fatal(err)
	}
	line := <-lines
	if line == "" {
		t.Fatal(line)
	}
	_ = io.EOF
}
