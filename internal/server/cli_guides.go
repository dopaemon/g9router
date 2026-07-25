package server

import "net/http"

func (s *Server) cliToolsGuidesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": []map[string]any{
		{"id": "claude", "name": "Claude Code", "description": "Anthropic Claude Code CLI", "configType": "env"},
		{"id": "openclaw", "name": "Open Claw", "description": "Open Claw AI Assistant", "configType": "custom"},
		{"id": "codex", "name": "OpenAI Codex CLI / App", "description": "OpenAI Codex CLI", "configType": "custom"},
		{"id": "opencode", "name": "OpenCode", "description": "OpenCode AI Terminal Assistant", "configType": "custom"},
		{"id": "cowork", "name": "Claude Cowork", "description": "Claude Desktop Cowork (third-party inference)", "configType": "custom"},
		{"id": "hermes", "name": "Hermes Agent", "description": "Nous Research self-improving AI agent", "configType": "custom"},
		{"id": "droid", "name": "Factory Droid", "description": "Factory Droid AI Assistant", "configType": "custom"},
		{"id": "cursor", "name": "Cursor", "description": "Cursor AI Code Editor", "configType": "guide"},
		{"id": "cline", "name": "Cline", "description": "Cline AI Coding Assistant", "configType": "custom"},
		{"id": "kilo", "name": "Kilo Code", "description": "Kilo Code AI Assistant", "configType": "custom"},
		{"id": "deepseek-tui", "name": "DeepSeek TUI", "description": "DeepSeek Terminal Coding Agent (Rust TUI)", "configType": "custom"},
		{"id": "grok-build", "name": "Grok Build", "description": "Grok Build coding assistant", "configType": "custom"},
		{"id": "jcode", "name": "JCode", "description": "JCode coding assistant", "configType": "custom"},
		{"id": "copilot", "name": "GitHub Copilot", "description": "GitHub Copilot custom model configuration", "configType": "custom"},
	}})
}
