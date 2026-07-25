package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"g9router/internal/providers"
)

func (s *Server) providerAccountsAPI(w http.ResponseWriter, r *http.Request, providerID string) {
	provider, ok := s.store.Find(providerID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		accounts := make([]providers.Account, len(provider.Accounts))
		copy(accounts, provider.Accounts)
		for index := range accounts {
			accounts[index].APIKey = ""
		}
		writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts})
	case http.MethodPost:
		var input struct {
			ID      string `json:"id"`
			APIKey  string `json:"apiKey"`
			Name    string `json:"name"`
			Email   string `json:"email"`
			Enabled *bool  `json:"enabled"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || strings.TrimSpace(input.APIKey) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "apiKey is required"})
			return
		}
		if input.ID == "" {
			input.ID = "account-" + time.Now().UTC().Format("20060102150405.000000000")
		}
		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		provider.Accounts = append(provider.Accounts, providers.Account{ID: input.ID, APIKey: input.APIKey, Name: input.Name, Email: input.Email, Enabled: enabled})
		if err := s.store.Upsert(provider); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": input.ID})
	case http.MethodPut:
		var input struct {
			ID      string `json:"id"`
			Enabled *bool  `json:"enabled"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input) != nil || input.ID == "" || input.Enabled == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id and enabled are required"})
			return
		}
		updated := false
		for index := range provider.Accounts {
			if provider.Accounts[index].ID == input.ID {
				provider.Accounts[index].Enabled = *input.Enabled
				updated = true
				break
			}
		}
		if !updated {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "account not found"})
			return
		}
		if err := s.store.Upsert(provider); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case http.MethodDelete:
		accountID := r.URL.Query().Get("id")
		filtered := provider.Accounts[:0]
		removed := false
		for _, account := range provider.Accounts {
			if account.ID == accountID {
				removed = true
				continue
			}
			filtered = append(filtered, account)
		}
		if !removed {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "account not found"})
			return
		}
		provider.Accounts = filtered
		if err := s.store.Upsert(provider); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
