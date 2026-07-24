package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"g9router/internal/proxypools"
)

const vercelRelaySource = `export const config={runtime:"edge"};export default async function handler(req){const target=req.headers.get("x-relay-target");const path=req.headers.get("x-relay-path")||"/";if(!target)return new Response(JSON.stringify({error:"Missing x-relay-target header"}),{status:400,headers:{"content-type":"application/json"}});const headers=new Headers(req.headers);headers.delete("x-relay-target");headers.delete("x-relay-path");headers.delete("host");const response=await fetch(target.replace(/\/$/,"")+path,{method:req.method,headers,body:req.method!=="GET"&&req.method!=="HEAD"?req.body:undefined});return new Response(response.body,{status:response.status,headers:response.headers})}`

const cloudflareRelaySource = `export default {async fetch(request){const target=request.headers.get("x-relay-target");const path=request.headers.get("x-relay-path")||"/";if(!target)return new Response(JSON.stringify({error:"Missing x-relay-target header"}),{status:400});const headers=new Headers(request.headers);headers.delete("x-relay-target");headers.delete("x-relay-path");headers.delete("host");try{return await fetch(target.replace(/\/$/,"")+path,{method:request.method,headers,body:request.method!=="GET"&&request.method!=="HEAD"?request.body:undefined})}catch(error){return new Response(JSON.stringify({error:error.message}),{status:502})}}}`

const denoRelaySource = `Deno.serve(async(request)=>{const target=request.headers.get("x-relay-target");const path=request.headers.get("x-relay-path")||"/";if(!target)return new Response(JSON.stringify({error:"Missing x-relay-target header"}),{status:400});const headers=new Headers(request.headers);headers.delete("x-relay-target");headers.delete("x-relay-path");headers.delete("host");try{return await fetch(target.replace(/\/$/,"")+path,{method:request.method,headers,body:request.method!=="GET"&&request.method!=="HEAD"?request.body:undefined})}catch(error){return new Response(JSON.stringify({error:error.message}),{status:502})}})`

func (s *Server) denoDeployAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Token     string `json:"denoToken"`
		OrgDomain string `json:"orgDomain"`
		Project   string `json:"projectName"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.OrgDomain) == "" || strings.TrimSpace(input.Token) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Organization domain and Deno Deploy API token are required"})
		return
	}
	if strings.TrimSpace(input.Project) == "" {
		input.Project = "relay-" + fmt.Sprint(time.Now().Unix())
	}
	headers := map[string]string{"Authorization": "Bearer " + input.Token, "Content-Type": "application/json"}
	createPayload := map[string]any{"slug": input.Project, "labels": map[string]string{"custom.kind": "9router-relay"}, "config": map[string]any{"install": "deno install", "runtime": map[string]string{"type": "dynamic", "entrypoint": "main.ts"}}}
	data, _ := json.Marshal(createPayload)
	appData, status, err := s.denoRequest(r.Context(), http.MethodPost, "https://api.deno.com/v2/apps", headers, data)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	var app struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(appData, &app) != nil || app.ID == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid Deno app response"})
		return
	}
	deployPayload := map[string]any{"assets": map[string]any{"main.ts": map[string]string{"kind": "file", "content": denoRelaySource, "encoding": "utf-8"}}}
	data, _ = json.Marshal(deployPayload)
	revisionData, status, err := s.denoRequest(r.Context(), http.MethodPost, "https://api.deno.com/v2/apps/"+app.ID+"/deploy", headers, data)
	if err != nil {
		_ = s.denoDelete(r.Context(), app.ID, input.Token)
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	var revision struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	_ = json.Unmarshal(revisionData, &revision)
	state := revision.Status
	for attempts := 0; (state == "queued" || state == "building") && attempts < 30; attempts++ {
		select {
		case <-r.Context().Done():
			_ = s.denoDelete(r.Context(), app.ID, input.Token)
			writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": r.Context().Err().Error()})
			return
		case <-time.After(2 * time.Second):
		}
		current, _, pollErr := s.denoRequest(r.Context(), http.MethodGet, "https://api.deno.com/v2/revisions/"+revision.ID, headers, nil)
		if pollErr != nil {
			break
		}
		var item struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(current, &item)
		state = item.Status
	}
	if state != "succeeded" {
		_ = s.denoDelete(r.Context(), app.ID, input.Token)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "deploy failed with status: " + state})
		return
	}
	org := strings.Split(input.OrgDomain, ".")[0]
	deployURL := "https://" + input.Project + "." + org + ".deno.net"
	pool, err := s.proxyPools.Create(proxypools.Pool{Name: input.Project, ProxyURL: deployURL, Type: "deno", IsActive: true})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"proxyPool": pool, "deployUrl": deployURL})
}

func (s *Server) denoRequest(ctx context.Context, method, endpoint string, headers map[string]string, body []byte) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, response.StatusCode, fmt.Errorf("Deno API %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, response.StatusCode, nil
}

func (s *Server) denoDelete(ctx context.Context, id, token string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, "https://api.deno.com/v2/apps/"+id, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := s.client.Do(request)
	if err == nil {
		response.Body.Close()
	}
	return err
}

func (s *Server) cloudflareDeployAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		AccountID string `json:"accountId"`
		Token     string `json:"apiToken"`
		Project   string `json:"projectName"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.AccountID) == "" || strings.TrimSpace(input.Token) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Cloudflare Account ID and API Token are required"})
		return
	}
	if strings.TrimSpace(input.Project) == "" {
		input.Project = "relay-" + fmt.Sprint(time.Now().Unix())
	}
	endpoint := "https://api.cloudflare.com/client/v4/accounts/" + input.AccountID + "/workers/scripts/" + input.Project
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("index.js", "index.js")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_, _ = part.Write([]byte(cloudflareRelaySource))
	meta, _ := writer.CreateFormField("metadata")
	metadata, _ := json.Marshal(map[string]any{"main_module": "index.js", "compatibility_date": "2024-03-20", "observability": map[string]bool{"enabled": true}})
	_, _ = meta.Write(metadata)
	_ = writer.Close()
	request, _ := http.NewRequestWithContext(r.Context(), http.MethodPut, endpoint, &body)
	request.Header.Set("Authorization", "Bearer "+input.Token)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := s.client.Do(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		writeJSON(w, response.StatusCode, map[string]string{"error": strings.TrimSpace(string(data))})
		return
	}
	subdomainReq, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://api.cloudflare.com/client/v4/accounts/"+input.AccountID+"/workers/subdomain", nil)
	subdomainReq.Header.Set("Authorization", "Bearer "+input.Token)
	subdomain, err := s.client.Do(subdomainReq)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer subdomain.Body.Close()
	subData, _ := io.ReadAll(io.LimitReader(subdomain.Body, 1<<20))
	var result struct {
		Result struct {
			Subdomain string `json:"subdomain"`
		} `json:"result"`
	}
	_ = json.Unmarshal(subData, &result)
	if result.Result.Subdomain == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "workers.dev subdomain is not configured"})
		return
	}
	deployURL := "https://" + input.Project + "." + result.Result.Subdomain + ".workers.dev"
	pool, err := s.proxyPools.Create(proxypools.Pool{Name: input.Project, ProxyURL: deployURL, Type: "cloudflare", IsActive: true})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"proxyPool": pool, "deployUrl": deployURL})
}

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
