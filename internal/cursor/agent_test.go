package cursor

import (
	"encoding/binary"
	"testing"
)

func TestAgentBodyIsConnectRPCFrame(t *testing.T) {
	frame := AgentBody([]map[string]string{{"role": "system", "content": "rules"}, {"role": "user", "content": "hello"}}, "default")
	if len(frame) < 5 || frame[0] != 0 || int(binary.BigEndian.Uint32(frame[1:5])) != len(frame)-5 {
		t.Fatalf("invalid frame: %x", frame)
	}
}

func TestParseAgentFrames(t *testing.T) {
	payload := agentMessage(1, agentMessage(1, agentString(1, "hello")))
	frame := agentFrame(payload)
	remaining, events, err := ParseAgentFrames(frame)
	if err != nil || len(remaining) != 0 || len(events) != 1 || events[0].Text != "hello" {
		t.Fatalf("parse = %x %#v %v", remaining, events, err)
	}
}
