package mitm

import (
	"crypto/tls"
	"os"
	"testing"
)

func TestEnsureCertificate(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := dir+"/ca.crt", dir+"/ca.key"
	if err := ensureCertificate(certFile, keyFile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(certFile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		t.Fatal(err)
	}
	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		t.Fatal(err)
	}
}
