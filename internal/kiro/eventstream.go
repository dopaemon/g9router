package kiro

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

type Event struct {
	Headers map[string]string
	Payload []byte
}

func Parse(buffer []byte) ([]byte, []Event, error) {
	remaining := buffer
	events := make([]Event, 0)
	for len(remaining) >= 12 {
		total := int(binary.BigEndian.Uint32(remaining[:4]))
		headerLength := int(binary.BigEndian.Uint32(remaining[4:8]))
		if total < 16 || headerLength < 0 || headerLength > total-16 {
			return nil, nil, fmt.Errorf("invalid eventstream frame lengths")
		}
		if len(remaining) < total {
			break
		}
		frame := remaining[:total]
		if crc32.ChecksumIEEE(frame[:8]) != binary.BigEndian.Uint32(frame[8:12]) {
			return nil, nil, fmt.Errorf("invalid eventstream prelude crc")
		}
		if crc32.ChecksumIEEE(frame[:total-4]) != binary.BigEndian.Uint32(frame[total-4:]) {
			return nil, nil, fmt.Errorf("invalid eventstream message crc")
		}
		headers, err := parseHeaders(frame[12 : 12+headerLength])
		if err != nil {
			return nil, nil, err
		}
		payloadEnd := total - 4
		events = append(events, Event{Headers: headers, Payload: append([]byte(nil), frame[12+headerLength:payloadEnd]...)})
		remaining = remaining[total:]
	}
	return remaining, events, nil
}

func parseHeaders(raw []byte) (map[string]string, error) {
	result := map[string]string{}
	for offset := 0; offset < len(raw); {
		if offset+2 > len(raw) {
			return nil, fmt.Errorf("truncated eventstream header")
		}
		nameLength := int(raw[offset])
		offset++
		if offset+nameLength+1 > len(raw) {
			return nil, fmt.Errorf("truncated eventstream header name")
		}
		name := string(raw[offset : offset+nameLength])
		offset += nameLength
		typeID := raw[offset]
		offset++
		switch typeID {
		case 7:
			if offset+2 > len(raw) {
				return nil, fmt.Errorf("truncated eventstream string header")
			}
			length := int(binary.BigEndian.Uint16(raw[offset : offset+2]))
			offset += 2
			if offset+length > len(raw) {
				return nil, fmt.Errorf("truncated eventstream string value")
			}
			result[name] = string(raw[offset : offset+length])
			offset += length
		case 6:
			if offset+1 > len(raw) {
				return nil, fmt.Errorf("truncated eventstream bool header")
			}
			result[name] = fmt.Sprintf("%t", raw[offset] != 0)
			offset++
		default:
			width := map[byte]int{0: 1, 1: 1, 2: 2, 3: 4, 4: 8, 5: 8, 8: 16}[typeID]
			if width == 0 || offset+width > len(raw) {
				return nil, fmt.Errorf("unsupported eventstream header type %d", typeID)
			}
			offset += width
		}
	}
	return result, nil
}
