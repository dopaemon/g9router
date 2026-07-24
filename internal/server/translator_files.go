package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

var translatorFiles = map[string]bool{
	"1_req_client.json": true, "2_req_source.json": true, "3_req_openai.json": true,
	"4_req_target.json": true, "5_res_provider.txt": true, "6_res_openai.txt": true,
	"7_res_client.txt": true, "7_res_client.json": true,
}

func translatorFilePath(file string) (string, bool) {
	if !translatorFiles[file] {
		return "", false
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	return filepath.Join(workingDir, "logs", "translator", file), true
}

func (s *Server) translatorLoadAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	file, ok := translatorFilePath(r.URL.Query().Get("file"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "Invalid file name"})
		return
	}
	content, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		writeJSON(w, http.StatusNotFound, map[string]any{"success": false, "error": "File not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "content": string(content)})
}

func (s *Server) translatorSaveAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		File    string `json:"file"`
		Content string `json:"content"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(&input) != nil || input.File == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "File and content required"})
		return
	}
	file, ok := translatorFilePath(input.File)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "Invalid file name"})
		return
	}
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	if err := os.WriteFile(file, []byte(input.Content), 0600); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}
