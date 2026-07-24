package cursor

import (
	"bytes"
	"testing"
)

func TestBodyUsesConnectFrame(t *testing.T) {
	body := Body([]map[string]string{{"role": "user", "content": "hello"}}, "default", false)
	if len(body) < 6 || body[0] != 0 {
		t.Fatalf("invalid frame: %x", body)
	}
	decoded, consumed, ok := ParseFrame(body)
	if !ok || consumed != len(body) || decoded.Text != "" {
		t.Fatalf("unexpected frame: %+v %d %v", decoded, consumed, ok)
	}
}

func TestHeadersStripCursorTokenPrefix(t *testing.T) {
	first := Headers("token", "machine", true)
	second := Headers("prefix::token", "machine", true)
	if first["authorization"] != "Bearer token" || second["authorization"] != "Bearer token" {
		t.Fatal(first["authorization"], second["authorization"])
	}
	if !bytes.HasSuffix([]byte(first["x-cursor-checksum"]), []byte("machine")) {
		t.Fatal(first["x-cursor-checksum"])
	}
}
