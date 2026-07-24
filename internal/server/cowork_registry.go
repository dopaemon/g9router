package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const coworkRegistryURL = "https://api.anthropic.com/mcp-registry/v0/servers"

var coworkRegistryCache struct {
	sync.RWMutex
	ts      time.Time
	servers []coworkMCPServer
}

type coworkMCPServer struct {
	Name        string   `json:"name"`
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	URL         string   `json:"url"`
	Transport   string   `json:"transport"`
	OAuth       bool     `json:"oauth"`
	ToolNames   []string `json:"toolNames"`
	ToolCount   int      `json:"toolCount"`
	IconURL     string   `json:"iconUrl,omitempty"`
}

func (s *Server) coworkMCPRegistryAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	force := r.URL.Query().Get("refresh") == "1"
	if !force {
		coworkRegistryCache.RLock()
		if time.Since(coworkRegistryCache.ts) < time.Hour && coworkRegistryCache.servers != nil {
			servers := append([]coworkMCPServer(nil), coworkRegistryCache.servers...)
			coworkRegistryCache.RUnlock()
			writeJSON(w, http.StatusOK, map[string]any{"cached": true, "servers": servers, "total": len(servers)})
			return
		}
		coworkRegistryCache.RUnlock()
	}
	servers, err := s.fetchCoworkMCPRegistry(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "servers": []coworkMCPServer{}, "total": 0})
		return
	}
	coworkRegistryCache.Lock()
	coworkRegistryCache.ts, coworkRegistryCache.servers = time.Now(), servers
	coworkRegistryCache.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"cached": false, "servers": servers, "total": len(servers)})
}

func directCoworkURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || strings.ContainsAny(raw, "<{") {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host != "mcp.claude.com" && !strings.HasSuffix(host, ".mcp.claude.com") && host != "api.anthropic.com"
}

func (s *Server) fetchCoworkMCPRegistry(parent context.Context) ([]coworkMCPServer, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	seen := map[string]bool{}
	servers := make([]coworkMCPServer, 0)
	cursor := ""
	for page := 0; page < 20; page++ {
		endpoint := coworkRegistryURL + "?limit=500&visibility=commercial,gsuite,gsuite-google"
		if cursor != "" {
			endpoint += "&cursor=" + url.QueryEscape(cursor)
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Accept", "application/json")
		response, err := s.client.Do(request)
		if err != nil {
			return nil, err
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			status := response.StatusCode
			response.Body.Close()
			return nil, fmt.Errorf("registry returned %d", status)
		}
		var pageData struct {
			Servers []struct {
				Server struct {
					Name        string `json:"name"`
					Title       string `json:"title"`
					Description string `json:"description"`
					Remotes     []struct {
						URL  string `json:"url"`
						Type string `json:"type"`
					} `json:"remotes"`
				} `json:"server"`
				Meta map[string]struct {
					Slug           string   `json:"slug"`
					DisplayName    string   `json:"displayName"`
					OneLiner       string   `json:"oneLiner"`
					RequiredFields []any    `json:"requiredFields"`
					ToolNames      []string `json:"toolNames"`
					IsAuthless     bool     `json:"isAuthless"`
					IconURL        string   `json:"iconUrl"`
				} `json:"_meta"`
			} `json:"servers"`
			Metadata struct {
				NextCursor string `json:"nextCursor"`
			} `json:"metadata"`
		}
		err = json.NewDecoder(response.Body).Decode(&pageData)
		response.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, item := range pageData.Servers {
			if len(item.Server.Remotes) == 0 {
				continue
			}
			remote := item.Server.Remotes[0]
			if !directCoworkURL(remote.URL) {
				continue
			}
			meta := item.Meta["com.anthropic.api/mcp-registry"]
			if len(meta.RequiredFields) > 0 || seen[remote.URL] {
				continue
			}
			seen[remote.URL] = true
			name, title := item.Server.Name, item.Server.Title
			if title == "" {
				title = meta.DisplayName
			}
			if title == "" {
				title = name
			}
			description := item.Server.Description
			if description == "" {
				description = meta.OneLiner
			}
			slug := meta.Slug
			if slug == "" {
				slug = name
			}
			transport := "http"
			if remote.Type == "sse" {
				transport = "sse"
			}
			servers = append(servers, coworkMCPServer{Name: name, Slug: slug, Title: title, Description: description, URL: remote.URL, Transport: transport, OAuth: !meta.IsAuthless, ToolNames: meta.ToolNames, ToolCount: len(meta.ToolNames), IconURL: meta.IconURL})
		}
		cursor = pageData.Metadata.NextCursor
		if cursor == "" {
			break
		}
	}
	return servers, nil
}
