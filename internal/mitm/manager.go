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
	"os/exec"
	"path/filepath"
	"runtime"
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

var toolHosts = map[string][]string{"antigravity": {"daily-cloudcode-pa.googleapis.com", "cloudcode-pa.googleapis.com"}, "copilot": {"api.individual.githubcopilot.com"}, "kiro": {"runtime.us-east-1.kiro.dev", "q.us-east-1.amazonaws.com", "codewhisperer.us-east-1.amazonaws.com"}, "cursor": {"api2.cursor.sh"}}

func New() *Manager { return &Manager{} }

func (m *Manager) Status() Status { m.mu.Lock(); defer m.mu.Unlock(); return m.status }

func (m *Manager) Start(baseURL, apiKey string) (Status, error) {
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
	originalDirector := proxy.Director
	proxy.Director = func(request *http.Request) {
		originalDirector(request)
		if apiKey != "" && request.Header.Get("Authorization") == "" {
			request.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}
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

func (m *Manager) DNSStatus() map[string]bool {
	data, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return map[string]bool{}
	}
	text := string(data)
	result := map[string]bool{}
	for tool, hosts := range toolHosts {
		result[tool] = true
		for _, host := range hosts {
			if !strings.Contains(text, "127.0.0.1 "+host) {
				result[tool] = false
			}
		}
	}
	return result
}

func (m *Manager) SetDNS(tool, password string, enabled bool) error {
	hosts, ok := toolHosts[tool]
	if !ok {
		return fmt.Errorf("unknown MITM tool: %s", tool)
	}
	if password != "" && strings.ContainsAny(password, "\r\n") {
		return fmt.Errorf("invalid sudo password")
	}
	data, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines)+len(hosts))
	for _, line := range lines {
		remove := false
		for _, host := range hosts {
			if strings.Contains(line, host) {
				remove = true
				break
			}
		}
		if !remove {
			kept = append(kept, line)
		}
	}
	if enabled {
		for _, host := range hosts {
			kept = append(kept, "127.0.0.1 "+host+" # g9router-mitm")
		}
	}
	next := strings.TrimRight(strings.Join(kept, "\n"), "\n") + "\n"
	if os.Geteuid() == 0 {
		return os.WriteFile("/etc/hosts", []byte(next), 0644)
	}
	if password == "" {
		return fmt.Errorf("root or sudo password required")
	}
	command := exec.Command("sudo", "-S", "tee", "/etc/hosts")
	command.Stdin = strings.NewReader(password + "\n" + next)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sudo hosts update failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *Manager) TrustCertificate(password string) error {
	certFile, _, err := m.certificateFiles()
	if err != nil {
		return err
	}
	if err := ensureCertificate(certFile, filepath.Join(filepath.Dir(certFile), "ca.key")); err != nil {
		return err
	}
	if password != "" && strings.ContainsAny(password, "\r\n") {
		return fmt.Errorf("invalid sudo password")
	}
	if runtime.GOOS == "windows" {
		if err := m.privileged(password, "certutil", "-addstore", "-f", "Root", certFile); err != nil {
			return err
		}
		m.mu.Lock()
		m.status.CertTrusted = true
		m.mu.Unlock()
		return nil
	}
	if runtime.GOOS == "darwin" {
		if err := m.privileged(password, "security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", certFile); err != nil {
			return err
		}
		m.mu.Lock()
		m.status.CertTrusted = true
		m.mu.Unlock()
		return nil
	}
	installed := "/usr/local/share/ca-certificates/g9router-mitm.crt"
	if err := m.privileged(password, "cp", certFile, installed); err != nil {
		return err
	}
	if err := m.privileged(password, "update-ca-certificates"); err != nil {
		return err
	}
	m.mu.Lock()
	m.status.CertTrusted = true
	m.mu.Unlock()
	return nil
}

func (m *Manager) privileged(password string, name string, args ...string) error {
	commandName, commandArgs := name, args
	if os.Geteuid() != 0 && runtime.GOOS != "windows" {
		commandName, commandArgs = "sudo", append([]string{"-S", name}, args...)
	}
	command := exec.Command(commandName, commandArgs...)
	if password != "" && commandName == "sudo" {
		command.Stdin = strings.NewReader(password + "\n")
	}
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %s", name, strings.TrimSpace(string(output)))
	}
	return nil
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
