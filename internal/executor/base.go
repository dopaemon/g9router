package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"g9router/internal/providers"
)

type Config struct {
	Provider      string
	BaseURLs      []string
	Format        string
	Headers       map[string]string
	APIKey        string
	RetryAttempts int
	RetryDelay    time.Duration
}
type Result struct {
	Status int
	Header http.Header
	Body   []byte
}

func ConfigForProvider(provider string, apiKey string) (Config, bool) {
	descriptor, ok := providers.Lookup(provider)
	if !ok {
		return Config{}, false
	}
	return Config{Provider: provider, BaseURLs: []string{descriptor.BaseURL}, Format: descriptor.Format, Headers: descriptor.Headers, APIKey: apiKey, RetryAttempts: 2, RetryDelay: 250 * time.Millisecond}, true
}

func Execute(ctx context.Context, client *http.Client, config Config, path string, body []byte, stream bool) (Result, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	attempts := config.RetryAttempts
	if attempts < 1 {
		attempts = 1
	}
	delay := config.RetryDelay
	if delay == 0 {
		delay = 250 * time.Millisecond
	}
	urls := config.BaseURLs
	if len(urls) == 0 {
		return Result{}, fmt.Errorf("no executor base URL")
	}
	var last error
	for index, base := range urls {
		for attempt := 0; attempt < attempts; attempt++ {
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+path, strings.NewReader(string(body)))
			if err != nil {
				return Result{}, err
			}
			request.Header.Set("Content-Type", "application/json")
			if stream {
				request.Header.Set("Accept", "text/event-stream")
			}
			for key, value := range config.Headers {
				request.Header.Set(key, value)
			}
			if config.APIKey != "" {
				if config.Format == "claude" {
					request.Header.Set("x-api-key", config.APIKey)
					request.Header.Set("anthropic-version", "2023-06-01")
				} else {
					request.Header.Set("Authorization", "Bearer "+config.APIKey)
				}
			}
			response, err := client.Do(request)
			if err != nil {
				last = err
				continue
			}
			resultBody, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr != nil {
				last = readErr
				continue
			}
			result := Result{Status: response.StatusCode, Header: response.Header, Body: resultBody}
			if response.StatusCode < 400 || (response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != http.StatusTooManyRequests) {
				return result, nil
			}
			last = fmt.Errorf("upstream status %d", response.StatusCode)
			if attempt+1 < attempts {
				select {
				case <-ctx.Done():
					return Result{}, ctx.Err()
				case <-time.After(delay):
				}
			}
		}
		if index+1 < len(urls) {
			continue
		}
	}
	return Result{}, last
}
