package cursor

import (
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"runtime"
	"time"
)

func hash(value, salt string) string {
	sum := sha256.Sum256([]byte(value + salt))
	return hex.EncodeToString(sum[:])
}

func sessionID(value string) string {
	namespace := []byte{0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}
	h := sha1.New()
	_, _ = h.Write(namespace)
	_, _ = h.Write([]byte(value))
	sum := h.Sum(nil)
	sum[6] = sum[6]&0x0f | 0x50
	sum[8] = sum[8]&0x3f | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s", hex.EncodeToString(sum[:4]), hex.EncodeToString(sum[4:6]), hex.EncodeToString(sum[6:8]), hex.EncodeToString(sum[8:10]), hex.EncodeToString(sum[10:16]))
}

func checksum(machineID string) string {
	timestamp := uint64(time.Now().UnixMilli() / 1000000)
	b := []byte{byte(timestamp >> 40), byte(timestamp >> 32), byte(timestamp >> 24), byte(timestamp >> 16), byte(timestamp >> 8), byte(timestamp)}
	key := byte(165)
	for index := range b {
		b[index] = (b[index] ^ key) + byte(index)
		key = b[index]
	}
	return base64.RawURLEncoding.EncodeToString(b) + machineID
}

func Headers(accessToken, machineID string, ghostMode bool) map[string]string {
	if index := len(accessToken); index > 0 {
		for i := 0; i+1 < index; i++ {
			if accessToken[i:i+2] == "::" {
				accessToken = accessToken[i+2:]
				break
			}
		}
	}
	if machineID == "" {
		machineID = hash(accessToken, "machineId")
	}
	configID := uuid()
	requestID := uuid()
	traceID := uuid()
	osName := runtime.GOOS
	if osName == "darwin" {
		osName = "macos"
	}
	arch := runtime.GOARCH
	if arch == "arm64" {
		arch = "aarch64"
	}
	return map[string]string{
		"authorization": "Bearer " + accessToken, "connect-accept-encoding": "gzip", "connect-protocol-version": "1", "content-type": "application/connect+proto", "user-agent": "connect-es/1.6.1",
		"x-amzn-trace-id": "Root=" + traceID, "x-client-key": hash(accessToken, ""), "x-cursor-checksum": checksum(machineID), "x-cursor-client-version": "3.12.17", "x-cursor-client-commit": "0fb762053c34788bb7760d5673f8a6d4c8589d50", "x-cursor-client-type": "ide", "x-cursor-client-os": osName, "x-cursor-client-arch": arch, "x-cursor-client-device-type": "desktop", "x-cursor-config-version": configID, "x-cursor-timezone": "UTC", "x-ghost-mode": fmt.Sprintf("%t", ghostMode), "x-request-id": requestID, "x-session-id": sessionID(accessToken),
	}
}

var _ = rand.Reader
