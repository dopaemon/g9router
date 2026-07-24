package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (s *Server) tavilySearch(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return false
	}
	var input struct {
		Model, Query, SearchType, Country string
		MaxResults                        int      `json:"max_results"`
		DomainFilter                      []string `json:"domain_filter"`
	}
	if json.Unmarshal(data, &input) != nil || strings.TrimSpace(input.Query) == "" {
		return false
	}
	for _, provider := range s.store.Resolve(input.Model) {
		if provider.ID != "tavily" {
			continue
		}
		if credential, ok := s.oauth.Get(provider.OAuthID); ok {
			provider.APIKey = credential.AccessToken
		}
		if provider.APIKey == "" {
			return false
		}
		maxResults := input.MaxResults
		if maxResults <= 0 {
			maxResults = 5
		}
		if maxResults > 20 {
			maxResults = 20
		}
		body := map[string]any{"query": strings.TrimSpace(input.Query), "max_results": maxResults, "topic": "general"}
		if input.SearchType == "news" {
			body["topic"] = "news"
		}
		if input.Country != "" {
			body["country"] = input.Country
		}
		includes, excludes := []string{}, []string{}
		for _, domain := range input.DomainFilter {
			if strings.HasPrefix(domain, "-") {
				excludes = append(excludes, strings.TrimPrefix(domain, "-"))
			} else {
				includes = append(includes, domain)
			}
		}
		if len(includes) > 0 {
			body["include_domains"] = includes
		}
		if len(excludes) > 0 {
			body["exclude_domains"] = excludes
		}
		encoded, _ := json.Marshal(body)
		request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://api.tavily.com/search", strings.NewReader(string(encoded)))
		if err != nil {
			return false
		}
		request.Header.Set("Authorization", "Bearer "+provider.APIKey)
		request.Header.Set("Content-Type", "application/json")
		response, err := s.client.Do(request)
		if err != nil {
			return false
		}
		defer response.Body.Close()
		responseData, _ := io.ReadAll(io.LimitReader(response.Body, 16<<20))
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return false
		}
		var payload struct {
			Results []struct {
				Title, URL, Content string
				Score               float64 `json:"score"`
				Published           string  `json:"published_date"`
			} `json:"results"`
		}
		if json.Unmarshal(responseData, &payload) != nil {
			return false
		}
		results := make([]map[string]any, 0, len(payload.Results))
		for index, result := range payload.Results {
			results = append(results, map[string]any{"title": result.Title, "url": result.URL, "snippet": result.Content, "position": index + 1, "score": result.Score, "published_at": result.Published, "citation": map[string]any{"provider": "tavily"}})
		}
		writeJSON(w, http.StatusOK, map[string]any{"provider": "tavily", "query": input.Query, "results": results, "answer": nil, "usage": map[string]any{"queries_used": 1}, "errors": []any{}})
		return true
	}
	return false
}

func (s *Server) braveSearch(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return false
	}
	var input struct {
		Model, Query, SearchType, Country, Language string
		MaxResults                                  int `json:"max_results"`
	}
	if json.Unmarshal(data, &input) != nil || strings.TrimSpace(input.Query) == "" {
		return false
	}
	for _, provider := range s.store.Resolve(input.Model) {
		if provider.ID != "brave-search" {
			continue
		}
		if credential, ok := s.oauth.Get(provider.OAuthID); ok {
			provider.APIKey = credential.AccessToken
		}
		if provider.APIKey == "" {
			return false
		}
		count := input.MaxResults
		if count <= 0 {
			count = 5
		}
		if count > 20 {
			count = 20
		}
		endpoint := "https://api.search.brave.com/res/v1/web/search"
		if input.SearchType == "news" {
			endpoint = "https://api.search.brave.com/res/v1/news/search"
		}
		query := url.Values{"q": {strings.TrimSpace(input.Query)}, "count": {strconv.Itoa(count)}}
		if input.Country != "" {
			query.Set("country", input.Country)
		}
		if input.Language != "" {
			query.Set("search_lang", input.Language)
		}
		request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint+"?"+query.Encode(), nil)
		if err != nil {
			return false
		}
		request.Header.Set("Accept", "application/json")
		request.Header.Set("X-Subscription-Token", provider.APIKey)
		response, err := s.client.Do(request)
		if err != nil {
			return false
		}
		defer response.Body.Close()
		responseData, _ := io.ReadAll(io.LimitReader(response.Body, 16<<20))
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return false
		}
		var payload struct {
			Web struct {
				Results []struct{ Title, URL, Description string } `json:"results"`
			} `json:"web"`
			News struct {
				Results []struct{ Title, URL, Description string } `json:"results"`
			} `json:"news"`
		}
		if json.Unmarshal(responseData, &payload) != nil {
			return false
		}
		raw := payload.Web.Results
		if input.SearchType == "news" {
			raw = payload.News.Results
		}
		results := make([]map[string]any, 0, len(raw))
		for index, result := range raw {
			results = append(results, map[string]any{"title": result.Title, "url": result.URL, "snippet": result.Description, "position": index + 1, "citation": map[string]any{"provider": "brave-search"}})
		}
		writeJSON(w, http.StatusOK, map[string]any{"provider": "brave-search", "query": input.Query, "results": results, "answer": nil, "usage": map[string]any{"queries_used": 1}, "errors": []any{}})
		return true
	}
	return false
}

func (s *Server) exaSearch(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return false
	}
	var input struct {
		Model, Query, SearchType string
		MaxResults               int `json:"max_results"`
	}
	if json.Unmarshal(data, &input) != nil || strings.TrimSpace(input.Query) == "" {
		return false
	}
	for _, provider := range s.store.Resolve(input.Model) {
		if provider.ID != "exa" {
			continue
		}
		if credential, ok := s.oauth.Get(provider.OAuthID); ok {
			provider.APIKey = credential.AccessToken
		}
		if provider.APIKey == "" {
			return false
		}
		count := input.MaxResults
		if count <= 0 {
			count = 5
		}
		if count > 20 {
			count = 20
		}
		body := map[string]any{
			"query":          strings.TrimSpace(input.Query),
			"numResults":     count,
			"type":           "auto",
			"text":           true,
			"highlights":     true,
			"includeDomains": []string{},
			"excludeDomains": []string{},
			"category":       "news",
		}
		encoded, _ := json.Marshal(body)
		request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://api.exa.ai/search", strings.NewReader(string(encoded)))
		if err != nil {
			return false
		}
		request.Header.Set("x-api-key", provider.APIKey)
		request.Header.Set("Content-Type", "application/json")
		response, err := s.client.Do(request)
		if err != nil {
			return false
		}
		defer response.Body.Close()
		responseData, _ := io.ReadAll(io.LimitReader(response.Body, 16<<20))
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return false
		}
		var payload struct {
			Results []struct {
				Title, URL, Text string
				Highlights       []string `json:"highlights"`
			} `json:"results"`
		}
		if json.Unmarshal(responseData, &payload) != nil {
			return false
		}
		results := make([]map[string]any, 0, len(payload.Results))
		for index, result := range payload.Results {
			snippet := result.Text
			if snippet == "" && len(result.Highlights) > 0 {
				snippet = strings.Join(result.Highlights, " ")
			}
			results = append(results, map[string]any{"title": result.Title, "url": result.URL, "snippet": snippet, "position": index + 1, "citation": map[string]any{"provider": "exa"}})
		}
		writeJSON(w, http.StatusOK, map[string]any{"provider": "exa", "query": input.Query, "results": results, "answer": nil, "usage": map[string]any{"queries_used": 1}, "errors": []any{}})
		return true
	}
	return false
}
