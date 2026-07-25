package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
)

type oauthProxySession struct {
	CodeVerifier string
	RedirectURI  string
	Status       string
	ConnectionID string
	Email        string
	Error        string
}

var oauthProxyState = struct {
	sync.Mutex
	sessions  map[string]*oauthProxySession
	listeners map[string]net.Listener
}{sessions: map[string]*oauthProxySession{}, listeners: map[string]net.Listener{}}

func (s *Server) oauthProxyAPI(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/oauth/"), "/"), "/")
	if len(parts) != 2 || r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	provider, action := parts[0], parts[1]
	if action == "start-proxy" {
		if provider != "codex" && provider != "xai" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Proxy only supported for codex/xai"})
			return
		}
		port, err := strconv.Atoi(r.URL.Query().Get("app_port"))
		if err != nil || port < 1 || port > 65535 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing app_port"})
			return
		}
		state, verifier, redirect := r.URL.Query().Get("state"), r.URL.Query().Get("code_verifier"), r.URL.Query().Get("redirect_uri")
		if state != "" && verifier != "" && redirect != "" {
			oauthProxyState.Lock()
			oauthProxyState.sessions[provider+":"+state] = &oauthProxySession{CodeVerifier: verifier, RedirectURI: redirect, Status: "pending"}
			oauthProxyState.Unlock()
		}
		ok, reason := startOAuthProxy(provider, port, s)
		if !ok {
			writeJSON(w, http.StatusOK, map[string]any{"success": false, "reason": reason, "serverSide": false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "serverSide": state != "" && verifier != "" && redirect != ""})
		return
	}
	if action == "stop-proxy" {
		if provider != "codex" && provider != "xai" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Proxy only supported for codex/xai"})
			return
		}
		stopOAuthProxy(provider)
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		return
	}
	if action == "poll-status" {
		if provider != "codex" && provider != "xai" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Poll only supported for codex/xai"})
			return
		}
		state := r.URL.Query().Get("state")
		if state == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing state"})
			return
		}
		oauthProxyState.Lock()
		session := oauthProxyState.sessions[provider+":"+state]
		if session == nil {
			oauthProxyState.Unlock()
			writeJSON(w, http.StatusOK, map[string]string{"status": "unknown"})
			return
		}
		payload := map[string]any{"status": session.Status}
		if session.ConnectionID != "" {
			payload["connectionId"] = session.ConnectionID
		}
		if session.Email != "" {
			payload["email"] = session.Email
		}
		if session.Error != "" {
			payload["error"] = session.Error
		}
		if session.Status == "done" || session.Status == "error" {
			delete(oauthProxyState.sessions, provider+":"+state)
		}
		oauthProxyState.Unlock()
		writeJSON(w, http.StatusOK, payload)
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Unknown action"})
}

func startOAuthProxy(provider string, appPort int, server *Server) (bool, string) {
	oauthProxyState.Lock()
	if oauthProxyState.listeners[provider] != nil {
		oauthProxyState.Unlock()
		return true, ""
	}
	oauthProxyState.Unlock()
	port := 1455
	if provider == "xai" {
		port = 56121
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false, "port_busy"
	}
	oauthProxyState.Lock()
	oauthProxyState.listeners[provider] = listener
	oauthProxyState.Unlock()
	go func() {
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go handleOAuthCallback(connection, provider, appPort, server)
		}
	}()
	return true, ""
}

func handleOAuthCallback(connection net.Conn, provider string, appPort int, server *Server) {
	defer connection.Close()
	request, err := http.ReadRequest(bufio.NewReader(connection))
	if err != nil {
		return
	}
	if request.URL.Path != "/callback" && request.URL.Path != "/auth/callback" {
		writeProxyResponse(connection, 404, "Not found")
		return
	}
	state, code := request.URL.Query().Get("state"), request.URL.Query().Get("code")
	oauthProxyState.Lock()
	session := oauthProxyState.sessions[provider+":"+state]
	oauthProxyState.Unlock()
	if session == nil {
		location := fmt.Sprintf("http://localhost:%d/callback%s", appPort, request.URL.RequestURI()[len(request.URL.Path):])
		writeRedirect(connection, location)
		stopOAuthProxy(provider)
		return
	}
	if request.URL.Query().Get("error") != "" || code == "" {
		updateOAuthProxySession(provider, state, "error", "", request.URL.Query().Get("error_description"))
		writeProxyResponse(connection, 200, "Authentication failed. You can close this window.")
		stopOAuthProxy(provider)
		return
	}
	body, _ := json.Marshal(map[string]string{"code": code, "state": state, "redirect_uri": session.RedirectURI, "code_verifier": session.CodeVerifier})
	exchangePath := "/api/oauth/" + provider + "/exchange"
	exchangeRequest := httptest.NewRequest(http.MethodPost, exchangePath, strings.NewReader(string(body)))
	exchangeRequest.Header.Set("Content-Type", "application/json")
	exchangeResponse := httptest.NewRecorder()
	if provider == "xai" {
		server.xaiExchangeAPI(exchangeResponse, exchangeRequest)
	} else {
		server.genericOAuthExchangeAPI(exchangeResponse, exchangeRequest)
	}
	var result map[string]any
	_ = json.NewDecoder(io.Reader(exchangeResponse.Body)).Decode(&result)
	if exchangeResponse.Code >= 300 {
		message := "Token exchange failed"
		if value, ok := result["error"].(string); ok {
			message = value
		}
		updateOAuthProxySession(provider, state, "error", "", message)
	} else {
		connectionData, _ := result["connection"].(map[string]any)
		connectionID, _ := connectionData["id"].(string)
		email, _ := connectionData["email"].(string)
		updateOAuthProxySession(provider, state, "done", connectionID, email)
	}
	writeProxyResponse(connection, 200, "Authentication processed. You can close this window.")
	stopOAuthProxy(provider)
}

func updateOAuthProxySession(provider, state, status, connectionID, message string) {
	oauthProxyState.Lock()
	defer oauthProxyState.Unlock()
	if session := oauthProxyState.sessions[provider+":"+state]; session != nil {
		session.Status, session.ConnectionID, session.Error = status, connectionID, message
	}
}

func stopOAuthProxy(provider string) {
	oauthProxyState.Lock()
	listener := oauthProxyState.listeners[provider]
	delete(oauthProxyState.listeners, provider)
	oauthProxyState.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
}

func writeRedirect(connection net.Conn, location string) {
	_, _ = fmt.Fprintf(connection, "HTTP/1.1 302 Found\r\nLocation: %s\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", location)
}
func writeProxyResponse(connection net.Conn, status int, message string) {
	body := []byte(message)
	statusText := http.StatusText(status)
	if statusText == "" {
		statusText = "Error"
	}
	_, _ = fmt.Fprintf(connection, "HTTP/1.1 %d %s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", status, statusText, len(body), body)
}
