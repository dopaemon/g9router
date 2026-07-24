package mitm

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Status struct {
	Running       bool   `json:"running"`
	PID           int    `json:"pid"`
	CertExists    bool   `json:"certExists"`
	CertTrusted   bool   `json:"certTrusted"`
	ListenAddress string `json:"listenAddress"`
	RouterBaseURL string `json:"routerBaseUrl"`
}

type Manager struct {
	mu                sync.Mutex
	server            *http.Server
	status            Status
	certFile, keyFile string
}

func New() *Manager { return &Manager{} }

func (m *Manager) Status() Status { m.mu.Lock(); defer m.mu.Unlock(); return m.status }

func (m *Manager) Start(baseURL string) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.server != nil {
		return m.status, nil
	}
	target, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || target.Scheme == "" || target.Host == "" {
		return Status{}, fmt.Errorf("invalid MITM router URL")
	}
	certFile, keyFile, err := m.certificateFiles()
	if err != nil {
		return Status{}, err
	}
	if err := ensureCertificate(certFile, keyFile); err != nil {
		return Status{}, err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
	server := &http.Server{Addr: "127.0.0.1:8443", Handler: proxy, ReadHeaderTimeout: 10 * time.Second}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return Status{}, err
	}
	listener, err := tls.Listen("tcp", server.Addr, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	if err != nil {
		return Status{}, err
	}
	m.server, m.certFile, m.keyFile = server, certFile, keyFile
	m.status = Status{Running: true, CertExists: true, ListenAddress: server.Addr, RouterBaseURL: target.String()}
	go func() {
		_ = server.Serve(listener)
		m.mu.Lock()
		if m.server == server {
			m.server = nil
			m.status.Running = false
		}
		m.mu.Unlock()
	}()
	return m.status, nil
}

func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	server := m.server
	m.server = nil
	m.status.Running = false
	m.mu.Unlock()
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

func (m *Manager) certificateFiles() (string, string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(root, "g9router", "mitm")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", "", err
	}
	return filepath.Join(dir, "ca.crt"), filepath.Join(dir, "ca.key"), nil
}

func ensureCertificate(certFile, keyFile string) error {
	if _, err := os.Stat(certFile); err == nil {
		if _, keyErr := os.Stat(keyFile); keyErr == nil {
			return nil
		}
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return err
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "g9router MITM"}, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(10, 0, 0), KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, DNSNames: []string{"localhost", "*.googleapis.com", "*.anthropic.com", "*.openai.com"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}
	certOut, err := os.OpenFile(certFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		return err
	}
	keyOut, err := os.OpenFile(keyFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	return pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}
