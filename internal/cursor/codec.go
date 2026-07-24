package cursor

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
)

const (
	wireVarint = 0
	wireBytes  = 2
)

func uuid() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return hex.EncodeToString(b[:4]) + "-" + hex.EncodeToString(b[4:6]) + "-" + hex.EncodeToString(b[6:8]) + "-" + hex.EncodeToString(b[8:10]) + "-" + hex.EncodeToString(b[10:])
}

func varint(value uint64) []byte {
	result := make([]byte, 0, 10)
	for value >= 0x80 {
		result = append(result, byte(value)|0x80)
		value >>= 7
	}
	return append(result, byte(value))
}

func field(number, kind uint64, value []byte) []byte {
	result := append(varint(number<<3|kind), varint(uint64(len(value)))...)
	return append(result, value...)
}

func stringField(number uint64, value string) []byte { return field(number, wireBytes, []byte(value)) }
func boolField(number uint64, value bool) []byte {
	if value {
		return append(varint(number<<3|wireVarint), 1)
	}
	return append(varint(number<<3|wireVarint), 0)
}
func intField(number, value uint64) []byte {
	return append(varint(number<<3|wireVarint), varint(value)...)
}

func join(parts ...[]byte) []byte {
	length := 0
	for _, part := range parts {
		length += len(part)
	}
	result := make([]byte, 0, length)
	for _, part := range parts {
		result = append(result, part...)
	}
	return result
}

func message(content string, role uint64, id string, agentic bool) []byte {
	return join(stringField(1, content), intField(2, role), stringField(13, id), boolField(29, agentic), intField(47, 1))
}

func model(name string) []byte { return stringField(1, name) }
func instruction() []byte      { return stringField(1, "") }
func metadata() []byte {
	return join(stringField(1, "linux"), stringField(2, "x64"), stringField(3, "3.12.17"), stringField(5, "UTC"))
}

func request(messages []map[string]string, modelName string, agentic bool) []byte {
	parts := make([][]byte, 0, len(messages)+12)
	ids := make([][]byte, 0, len(messages))
	for _, item := range messages {
		id := uuid()
		role := uint64(1)
		if item["role"] == "assistant" {
			role = 2
		}
		parts = append(parts, field(1, wireBytes, message(item["content"], role, id, agentic)))
		ids = append(ids, join(stringField(1, id), intField(3, role)))
	}
	parts = append(parts,
		intField(2, 1), field(3, wireBytes, instruction()), intField(4, 1), field(5, wireBytes, model(modelName)),
		field(8, wireBytes, nil), intField(13, 1), field(15, wireBytes, nil), intField(19, 1), stringField(23, uuid()),
		field(26, wireBytes, metadata()), boolField(27, agentic))
	for _, id := range ids {
		parts = append(parts, field(30, wireBytes, id))
	}
	parts = append(parts, intField(35, 0), intField(38, 0), intField(46, map[bool]uint64{false: 1, true: 2}[agentic]), field(47, wireBytes, nil), boolField(48, !agentic), intField(51, 0), intField(53, 1), stringField(54, map[bool]string{false: "Ask", true: "Agent"}[agentic]))
	return join(parts...)
}

func Body(messages []map[string]string, modelName string, agentic bool) []byte {
	payload := field(1, wireBytes, request(messages, modelName, agentic))
	frame := make([]byte, 5+len(payload))
	frame[0] = 0
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	return frame
}
