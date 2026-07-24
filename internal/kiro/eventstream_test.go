package kiro

import (
	"encoding/binary"
	"hash/crc32"
	"testing"
)

func TestParseEventStreamFrame(t *testing.T) {
	headers := []byte{11, ':', 'e', 'v', 'e', 'n', 't', '-', 't', 'y', 'p', 'e', 7, 0, 4, 't', 'e', 's', 't'}
	payload := []byte(`{"content":"hi"}`)
	total := 12 + len(headers) + len(payload) + 4
	frame := make([]byte, total)
	binary.BigEndian.PutUint32(frame[:4], uint32(total))
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(headers)))
	binary.BigEndian.PutUint32(frame[8:12], crc32.ChecksumIEEE(frame[:8]))
	copy(frame[12:], headers)
	copy(frame[12+len(headers):], payload)
	binary.BigEndian.PutUint32(frame[total-4:], crc32.ChecksumIEEE(frame[:total-4]))
	remaining, events, err := Parse(frame)
	if err != nil || len(remaining) != 0 || len(events) != 1 || events[0].Headers[":event-type"] != "test" || string(events[0].Payload) != string(payload) {
		t.Fatalf("parse = %x %#v %v", remaining, events, err)
	}
}
