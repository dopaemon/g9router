package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestSignAWSRequest(t *testing.T) {
	request, err := http.NewRequest(http.MethodPost, "https://polly.us-east-1.amazonaws.com/v1/speech", strings.NewReader(`{"Text":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Host", request.Host)
	if !signAWSRequest(request, []byte(`{"Text":"hello"}`), "AKID", "SECRET", "us-east-1", "polly") {
		t.Fatal("signing failed")
	}
	if !strings.HasPrefix(request.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=AKID/") {
		t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
	}
}
