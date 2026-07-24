package cursor

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
)

type Response struct {
	Text     string
	Thinking string
	ToolID   string
	ToolName string
	ToolArgs string
}

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

func readFields(payload []byte) map[uint64][][]byte {
	fields := map[uint64][][]byte{}
	for offset := 0; offset < len(payload); {
		tag, next := readVarint(payload, offset)
		if next <= offset {
			break
		}
		offset = next
		number, kind := tag>>3, tag&7
		if kind == wireVarint {
			_, offset = readVarint(payload, offset)
			continue
		}
		if kind != wireBytes {
			break
		}
		length, next := readVarint(payload, offset)
		if next <= offset || next+int(length) > len(payload) {
			break
		}
		offset = next
		fields[number] = append(fields[number], payload[offset:offset+int(length)])
		offset += int(length)
	}
	return fields
}

func readVarint(payload []byte, offset int) (uint64, int) {
	var value uint64
	for shift := uint(0); offset < len(payload) && shift < 64; shift += 7 {
		part := payload[offset]
		offset++
		value |= uint64(part&0x7f) << shift
		if part < 0x80 {
			return value, offset
		}
	}
	return 0, offset
}

func ParseFrame(frame []byte) (Response, int, bool) {
	if len(frame) < 5 {
		return Response{}, 0, false
	}
	length := int(binary.BigEndian.Uint32(frame[1:5]))
	if length < 0 || 5+length > len(frame) {
		return Response{}, 0, false
	}
	fields := readFields(frame[5 : 5+length])
	result := Response{}
	if values := fields[1]; len(values) > 0 {
		tool := readFields(values[0])
		if ids := tool[3]; len(ids) > 0 {
			result.ToolID = string(ids[0])
		}
		if names := tool[9]; len(names) > 0 {
			result.ToolName = string(names[0])
		}
		if args := tool[10]; len(args) > 0 {
			result.ToolArgs = string(args[0])
		}
	}
	if values := fields[2]; len(values) > 0 {
		response := readFields(values[0])
		if texts := response[1]; len(texts) > 0 {
			result.Text = string(texts[0])
		}
		if thinking := response[25]; len(thinking) > 0 {
			inner := readFields(thinking[0])
			if values := inner[1]; len(values) > 0 {
				result.Thinking = string(values[0])
			}
		}
	}
	return result, 5 + length, true
}
