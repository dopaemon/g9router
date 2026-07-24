package server

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const qoderRSAPublicKey = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDA8iMH5c02LilrsERw9t6Pv5Nc
4k6Pz1EaDicBMpdpxKduSZu5OANqUq8er4GM95omAGIOPOh+Nx0spthYA2BqGz+l
6HRkPJ7S236FZz73In/KVuLnwI8JJ2CbuJap8kvheCCZpmAWpb/cPx/3Vr/J6I17
XcW+ML9FoCI6AOvOzwIDAQAB
-----END PUBLIC KEY-----`

type qoderCOSYCredentials struct {
	UserID, AuthToken, Name, Email, MachineID string
}

func qoderCOSYHeaders(body []byte, requestURL string, credentials qoderCOSYCredentials) (map[string]string, error) {
	if credentials.UserID == "" || credentials.AuthToken == "" {
		return nil, fmt.Errorf("qoder cosy credentials are incomplete")
	}
	aesKey := uuid.New().String()[:16]
	infoPayload, err := json.Marshal(map[string]string{"uid": credentials.UserID, "security_oauth_token": credentials.AuthToken, "name": credentials.Name, "aid": "", "email": credentials.Email})
	if err != nil {
		return nil, err
	}
	info, err := qoderAESBase64(infoPayload, aesKey)
	if err != nil {
		return nil, err
	}
	publicKey, _ := pem.Decode([]byte(qoderRSAPublicKey))
	parsed, err := x509.ParsePKIXPublicKey(publicKey.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("qoder cosy key is not RSA")
	}
	wrappedKey, err := rsa.EncryptPKCS1v15(rand.Reader, rsaKey, []byte(aesKey))
	if err != nil {
		return nil, err
	}
	cosyKey := base64.StdEncoding.EncodeToString(wrappedKey)
	payloadJSON, err := json.Marshal(map[string]string{"version": "v1", "requestId": uuid.NewString(), "info": info, "cosyVersion": "1.0.0", "ideVersion": ""})
	if err != nil {
		return nil, err
	}
	payload := base64.StdEncoding.EncodeToString(payloadJSON)
	sigPath := qoderSigPath(requestURL)
	date := strconv.FormatInt(time.Now().Unix(), 10)
	signature := qoderMD5([]byte(payload + "\n" + cosyKey + "\n" + date + "\n" + string(body) + "\n" + sigPath))
	machineID := credentials.MachineID
	if machineID == "" {
		machineID = uuid.NewString()
	}
	return map[string]string{
		"Authorization":          "Bearer COSY." + payload + "." + signature,
		"Cosy-Key":               cosyKey,
		"Cosy-User":              credentials.UserID,
		"Cosy-Date":              date,
		"Cosy-Version":           "1.0.0",
		"Cosy-Machineid":         machineID,
		"Cosy-Machinetoken":      machineID,
		"Cosy-Machinetype":       "5",
		"Cosy-Machineos":         "x86_64_windows",
		"Cosy-Clienttype":        "5",
		"Cosy-Clientip":          "127.0.0.1",
		"Cosy-Bodyhash":          qoderMD5(body),
		"Cosy-Bodylength":        strconv.Itoa(len(body)),
		"Cosy-Sigpath":           sigPath,
		"Cosy-Data-Policy":       "disagree",
		"Cosy-Organization-Id":   "",
		"Cosy-Organization-Tags": "",
		"Login-Version":          "v2",
		"X-Request-Id":           uuid.NewString(),
	}, nil
}

func qoderAESBase64(plain []byte, key string) (string, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	padded := pkcs7Pad(plain, block.BlockSize())
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, []byte(key)).CryptBlocks(ciphertext, padded)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func pkcs7Pad(value []byte, size int) []byte {
	padding := size - len(value)%size
	return append(append([]byte(nil), value...), bytes.Repeat([]byte{byte(padding)}, padding)...)
}

func qoderMD5(value []byte) string {
	sum := md5.Sum(value)
	return hex.EncodeToString(sum[:])
}

func qoderSigPath(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	path := parsed.Path
	return strings.TrimPrefix(path, "/algo")
}
