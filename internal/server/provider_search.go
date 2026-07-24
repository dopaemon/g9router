package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"g9router/internal/providers"
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

func (s *Server) serperSearch(w http.ResponseWriter, r *http.Request) bool {
	return s.searchJSONProvider(w, r, "serper", func(input searchInput, provider providers.Provider) (*http.Request, error) {
		endpoint := "search"
		if input.SearchType == "news" {
			endpoint = "news"
		}
		body := map[string]any{"q": strings.TrimSpace(input.Query), "num": boundedSearchResults(input.MaxResults)}
		if input.Country != "" {
			body["gl"] = strings.ToLower(input.Country)
		}
		if input.Language != "" {
			body["hl"] = input.Language
		}
		return newSearchRequest(r, http.MethodPost, "https://google.serper.dev/"+endpoint, provider.APIKey, body, "X-API-Key")
	})
}

func (s *Server) googlePSESearch(w http.ResponseWriter, r *http.Request) bool {
	return s.searchJSONProvider(w, r, "google-pse", func(input searchInput, provider providers.Provider) (*http.Request, error) {
		cx := ""
		if value, ok := provider.ProviderSpecificData["cx"].(string); ok {
			cx = value
		}
		if cx == "" {
			return nil, nil
		}
		query := url.Values{"key": {provider.APIKey}, "cx": {cx}, "q": {strings.TrimSpace(input.Query)}, "num": {strconv.Itoa(minSearchResults(input.MaxResults, 10))}}
		if input.Country != "" {
			query.Set("gl", strings.ToLower(input.Country))
		}
		if input.Language != "" {
			query.Set("hl", input.Language)
		}
		request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://www.googleapis.com/customsearch/v1?"+query.Encode(), nil)
		if err == nil {
			request.Header.Set("Accept", "application/json")
		}
		return request, err
	})
}

func (s *Server) searxngSearch(w http.ResponseWriter, r *http.Request) bool {
	return s.searchJSONProvider(w, r, "searxng", func(input searchInput, provider providers.Provider) (*http.Request, error) {
		baseURL := strings.TrimRight(provider.BaseURL, "/")
		if !strings.HasSuffix(baseURL, "/search") {
			baseURL += "/search"
		}
		query := url.Values{"q": {strings.TrimSpace(input.Query)}, "format": {"json"}, "categories": {"general"}}
		if input.SearchType == "news" {
			query.Set("categories", "news")
		}
		if input.Language != "" {
			query.Set("language", input.Language)
		}
		request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, baseURL+"?"+query.Encode(), nil)
		if err == nil {
			request.Header.Set("Accept", "application/json")
		}
		return request, err
	})
}

func (s *Server) perplexitySearch(w http.ResponseWriter, r *http.Request) bool {
	return s.searchJSONProvider(w, r, "perplexity", func(input searchInput, provider providers.Provider) (*http.Request, error) {
		body := map[string]any{"query": strings.TrimSpace(input.Query), "max_results": boundedSearchResults(input.MaxResults)}
		if input.Country != "" {
			body["country"] = input.Country
		}
		if input.Language != "" {
			body["search_language_filter"] = []string{input.Language}
		}
		return newSearchRequest(r, http.MethodPost, "https://api.perplexity.ai/search", provider.APIKey, body, "Authorization")
	})
}

func (s *Server) linkupSearch(w http.ResponseWriter, r *http.Request) bool {
	return s.searchJSONProvider(w, r, "linkup", func(input searchInput, provider providers.Provider) (*http.Request, error) {
		body := map[string]any{"q": strings.TrimSpace(input.Query), "depth": "standard", "outputType": "searchResults", "maxResults": boundedSearchResults(input.MaxResults)}
		return newSearchRequest(r, http.MethodPost, "https://api.linkup.so/v1/search", provider.APIKey, body, "Authorization")
	})
}

func (s *Server) searchAPISearch(w http.ResponseWriter, r *http.Request) bool {
	return s.searchJSONProvider(w, r, "searchapi", func(input searchInput, provider providers.Provider) (*http.Request, error) {
		engine := "google"
		if input.SearchType == "news" {
			engine = "google_news"
		}
		query := url.Values{"engine": {engine}, "q": {strings.TrimSpace(input.Query)}, "api_key": {provider.APIKey}}
		if input.Country != "" {
			query.Set("gl", strings.ToLower(input.Country))
		}
		if input.Language != "" {
			query.Set("hl", input.Language)
		}
		request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://www.searchapi.io/api/v1/search?"+query.Encode(), nil)
		if err == nil {
			request.Header.Set("Accept", "application/json")
		}
		return request, err
	})
}

func (s *Server) youComSearch(w http.ResponseWriter, r *http.Request) bool {
	return s.searchJSONProvider(w, r, "youcom", func(input searchInput, provider providers.Provider) (*http.Request, error) {
		query := url.Values{"query": {strings.TrimSpace(input.Query)}, "count": {strconv.Itoa(boundedSearchResults(input.MaxResults))}}
		if input.Country != "" {
			query.Set("country", input.Country)
		}
		if input.Language != "" {
			query.Set("language", input.Language)
		}
		request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://ydc-index.io/v1/search?"+query.Encode(), nil)
		if err == nil {
			request.Header.Set("Accept", "application/json")
			request.Header.Set("X-API-Key", provider.APIKey)
		}
		return request, err
	})
}

type searchInput struct {
	Model, Query, SearchType, Country, Language string
	MaxResults                                  int `json:"max_results"`
}

func boundedSearchResults(value int) int { return minSearchResults(value, 20) }

func minSearchResults(value, maximum int) int {
	if value <= 0 {
		return 5
	}
	if value > maximum {
		return maximum
	}
	return value
}

func newSearchRequest(r *http.Request, method, endpoint, token string, body map[string]any, tokenHeader string) (*http.Request, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(r.Context(), method, endpoint, strings.NewReader(string(encoded)))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(tokenHeader, token)
	return request, nil
}

func (s *Server) searchJSONProvider(w http.ResponseWriter, r *http.Request, id string, build func(searchInput, providers.Provider) (*http.Request, error)) bool {
	if r.Method != http.MethodPost {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return false
	}
	var input searchInput
	if json.Unmarshal(data, &input) != nil || strings.TrimSpace(input.Query) == "" {
		return false
	}
	for _, provider := range s.store.Resolve(input.Model) {
		if provider.ID != id || (id != "searxng" && provider.APIKey == "") {
			continue
		}
		if credential, ok := s.oauth.Get(provider.OAuthID); ok {
			provider.APIKey = credential.AccessToken
		}
		request, err := build(input, provider)
		if err != nil || request == nil {
			return false
		}
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
			Items []struct {
				Title, URL, Link, Snippet, Description, Content string
				Published                                       string `json:"publishedDate"`
			} `json:"items"`
			Results []struct {
				Title, URL, Content, Snippet string
			} `json:"results"`
		}
		if json.Unmarshal(responseData, &payload) != nil {
			return false
		}
		results := make([]map[string]any, 0)
		if id == "searxng" {
			for index, item := range payload.Results {
				results = append(results, map[string]any{"title": item.Title, "url": item.URL, "snippet": firstSearchText(item.Content, item.Snippet), "position": index + 1, "citation": map[string]any{"provider": id}})
			}
		} else {
			for index, item := range payload.Items {
				results = append(results, map[string]any{"title": item.Title, "url": firstSearchText(item.Link, item.URL), "snippet": firstSearchText(item.Snippet, item.Description), "position": index + 1, "citation": map[string]any{"provider": id}})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"provider": id, "query": input.Query, "results": results, "answer": nil, "usage": map[string]any{"queries_used": 1}, "errors": []any{}})
		return true
	}
	return false
}

func firstSearchText(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
