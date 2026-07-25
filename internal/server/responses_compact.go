package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

func (s *Server) responsesCompactAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		setCORS(w, "GET, POST, OPTIONS")
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read request body"})
		return
	}
	var input map[string]any
	if json.Unmarshal(body, &input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	input["_compact"] = true
	body, _ = json.Marshal(input)
	clone := r.Clone(r.Context())
	clone.Body = io.NopCloser(strings.NewReader(string(body)))
	s.forwardJSON(w, clone, "/responses")
}
