package server

import "net/http"

func (s *Server) tagsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, []map[string]any{
		{"name": "llama3.2", "modified_at": "2025-12-26T00:00:00Z", "size": 2000000000, "digest": "abc123def456", "details": map[string]string{"format": "gguf", "family": "llama", "parameter_size": "3B", "quantization_level": "Q4_K_M"}},
		{"name": "qwen2.5", "modified_at": "2025-12-26T00:00:00Z", "size": 4000000000, "digest": "def456abc123", "details": map[string]string{"format": "gguf", "family": "qwen", "parameter_size": "7B", "quantization_level": "Q4_K_M"}},
	})
}
