package cursor

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
)

type AgentEvent struct {
	Text, Error          string
	Done, ContextRequest bool
}

func AgentBody(messages []map[string]string, modelName string) []byte {
	system := ""
	chat := make([]map[string]string, 0, len(messages))
	for _, message := range messages {
		if message["role"] == "system" {
			if system != "" {
				system += "\n\n"
			}
			system += message["content"]
			continue
		}
		chat = append(chat, message)
	}
	current := len(chat) - 1
	for index := len(chat) - 1; index >= 0; index-- {
		if chat[index]["role"] == "user" {
			current = index
			break
		}
	}
	history := make([][]byte, 0, current)
	for _, message := range chat[:max(0, current)] {
		content := message["content"]
		if content == "" {
			continue
		}
		role := uint64(1)
		if message["role"] == "assistant" {
			role = 2
		}
		history = append(history, agentMessage(2, agentMessage(1, agentMessage(1, agentMessage(1, agentString(1, content))))))
		_ = role
	}
	userText := "Continue."
	if current >= 0 && current < len(chat) && chat[current]["content"] != "" {
		userText = chat[current]["content"]
	}
	user := join(agentString(1, userText), agentString(2, randomUUID()))
	historyPayload := []byte(nil)
	if len(history) > 0 {
		historyPayload = join(history...)
	}
	userAction := agentMessage(1, user)
	if len(historyPayload) > 0 {
		userAction = join(userAction, agentMessage(7, historyPayload))
	}
	conversation := agentMessage(1, agentMessage(1, userAction))
	requestedModel := join(agentString(1, modelName), agentBool(7, true))
	run := agentMessage(1, []byte{})
	run = join(run, agentMessage(2, conversation))
	if system != "" {
		run = join(run, agentString(8, system))
	}
	run = join(run, agentMessage(9, requestedModel))
	return agentFrame(agentMessage(1, run))
}

func AgentContextResponse() []byte {
	success := agentMessage(1, []byte{})
	result := agentMessage(1, success)
	clientMessage := agentMessage(10, result)
	return agentFrame(agentMessage(2, clientMessage))
}

func ParseAgentFrames(buffer []byte) ([]byte, []AgentEvent, error) {
	pending := buffer
	events := make([]AgentEvent, 0)
	for len(pending) >= 5 {
		flags := pending[0]
		length := int(binary.BigEndian.Uint32(pending[1:5]))
		if length < 0 || len(pending) < 5+length {
			break
		}
		payload := pending[5 : 5+length]
		pending = pending[5+length:]
		if flags&1 != 0 {
			reader, err := gzip.NewReader(bytes.NewReader(payload))
			if err != nil {
				return pending, nil, err
			}
			payload, err = io.ReadAll(reader)
			_ = reader.Close()
			if err != nil {
				return pending, nil, err
			}
		}
		if flags&2 != 0 {
			continue
		}
		events = append(events, parseAgentMessage(payload))
	}
	return pending, events, nil
}

func parseAgentMessage(payload []byte) AgentEvent {
	fields := readFields(payload)
	if values := fields[1]; len(values) > 0 {
		update := readFields(values[0])
		if values := update[1]; len(values) > 0 {
			delta := readFields(values[0])
			if text := delta[1]; len(text) > 0 {
				return AgentEvent{Text: string(text[0])}
			}
		}
		if hasField(update, 14) {
			return AgentEvent{Done: true}
		}
	}
	if values := fields[2]; len(values) > 0 {
		request := readFields(values[0])
		if hasField(request, 10) {
			return AgentEvent{ContextRequest: true}
		}
	}
	return AgentEvent{}
}

func agentString(number uint64, value string) []byte  { return field(number, wireBytes, []byte(value)) }
func agentMessage(number uint64, value []byte) []byte { return field(number, wireBytes, value) }
func agentBool(number uint64, value bool) []byte {
	if value {
		return append(varint(number<<3|wireVarint), 1)
	}
	return append(varint(number<<3|wireVarint), 0)
}
func agentFrame(payload []byte) []byte {
	frame := make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	return frame
}
func hasField(fields map[uint64][][]byte, number uint64) bool { _, ok := fields[number]; return ok }
func randomUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%032x", 0)
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
