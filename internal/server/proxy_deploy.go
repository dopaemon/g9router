package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"g9router/internal/proxypools"
)

const vercelRelaySource = `export const config={runtime:"edge"};export default async function handler(req){const target=req.headers.get("x-relay-target");const path=req.headers.get("x-relay-path")||"/";if(!target)return new Response(JSON.stringify({error:"Missing x-relay-target header"}),{status:400,headers:{"content-type":"application/json"}});const headers=new Headers(req.headers);headers.delete("x-relay-target");headers.delete("x-relay-path");headers.delete("host");const response=await fetch(target.replace(/\/$/,"")+path,{method:req.method,headers,body:req.method!=="GET"&&req.method!=="HEAD"?req.body:undefined});return new Response(response.body,{status:response.status,headers:response.headers})}`

func (s *Server) vercelDeployAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Token   string `json:"vercelToken"`
		Project string `json:"projectName"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.Token) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Vercel API token is required"})
		return
	}
	if strings.TrimSpace(input.Project) == "" {
		input.Project = "relay-" + fmt.Sprint(time.Now().Unix())
	}
	payload := map[string]any{"name": input.Project, "files": []map[string]string{{"file": "api/relay.js", "data": vercelRelaySource}, {"file": "package.json", "data": fmt.Sprintf(`{"name":%q,"version":"1.0.0"}`, input.Project)}, {"file": "vercel.json", "data": `{"rewrites":[{"source":"/(.*)","destination":"/api/relay"}]}`}}, "projectSettings": map[string]any{"framework": nil}, "target": "production"}
	data, _ := json.Marshal(payload)
	response, err := s.vercelRequest(r.Context(), http.MethodPost, "https://api.vercel.com/v13/deployments", input.Token, data)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	var deployment struct {
		ID        string `json:"id"`
		UID       string `json:"uid"`
		ProjectID string `json:"projectId"`
		URL       string `json:"url"`
	}
	if json.Unmarshal(response, &deployment) != nil || deployment.ID == "" && deployment.UID == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid Vercel deployment response"})
		return
	}
	id := deployment.ID
	if id == "" {
		id = deployment.UID
	}
	ready, err := s.pollVercel(r.Context(), id, input.Token)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	var readyData struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(ready, &readyData)
	if readyData.URL == "" {
		readyData.URL = deployment.URL
	}
	pool, err := s.proxyPools.Create(proxypools.Pool{Name: input.Project, ProxyURL: "https://" + strings.TrimPrefix(readyData.URL, "https://"), Type: "vercel", IsActive: true})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"proxyPool": pool, "deployUrl": pool.ProxyURL})
}

func (s *Server) vercelRequest(ctx context.Context, method, endpoint, token string, body []byte) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Vercel API %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func (s *Server) pollVercel(ctx context.Context, id, token string) ([]byte, error) {
	deadline := time.NewTimer(2 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		data, err := s.vercelRequest(ctx, http.MethodGet, "https://api.vercel.com/v13/deployments/"+id, token, nil)
		if err != nil {
			return nil, err
		}
		var state struct {
			ReadyState string `json:"readyState"`
		}
		_ = json.Unmarshal(data, &state)
		if state.ReadyState == "READY" {
			return data, nil
		}
		if state.ReadyState == "ERROR" || state.ReadyState == "CANCELED" {
			return nil, fmt.Errorf("deployment failed: %s", state.ReadyState)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, fmt.Errorf("deployment timed out")
		case <-ticker.C:
		}
	}
}
