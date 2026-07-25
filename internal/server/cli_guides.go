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
		{"id": "roo", "name": "Roo", "description": "Roo AI Assistant", "configType": "guide", "guideSteps": []map[string]any{
			{"step": 1, "title": "Open Settings", "desc": "Go to Roo Settings panel"},
			{"step": 2, "title": "Select Provider", "desc": "Choose API Provider → Ollama"},
			{"step": 3, "title": "Base URL", "value": "{{baseUrl}}", "copyable": true},
			{"step": 4, "title": "API Key", "type": "apiKeySelector"},
			{"step": 5, "title": "Select Model", "type": "modelSelector"},
		}},
		{"id": "continue", "name": "Continue", "description": "Continue AI Assistant", "configType": "guide", "guideSteps": []map[string]any{
			{"step": 1, "title": "Open Config", "desc": "Open Continue configuration file"},
			{"step": 2, "title": "API Key", "type": "apiKeySelector"},
			{"step": 3, "title": "Select Model", "type": "modelSelector"},
			{"step": 4, "title": "Add Model Config", "desc": "Add the following configuration to your models array:"},
		}, "codeBlock": map[string]string{"language": "json", "code": "{\n  \"apiBase\": \"{{baseUrl}}\",\n  \"title\": \"{{model}}\",\n  \"model\": \"{{model}}\",\n  \"provider\": \"openai\",\n  \"apiKey\": \"{{apiKey}}\"\n}"}},
		{"id": "amp", "name": "Amp CLI", "description": "Sourcegraph Amp coding assistant CLI", "configType": "guide", "defaultCommand": "amp", "guideSteps": []map[string]any{
			{"step": 1, "title": "Install Amp", "desc": "Install the Amp CLI using the package manager supported by your environment."},
			{"step": 2, "title": "API Key", "type": "apiKeySelector"},
			{"step": 3, "title": "Base URL", "value": "{{baseUrl}}", "copyable": true},
			{"step": 4, "title": "Select Model", "type": "modelSelector"},
			{"step": 5, "title": "Add Shorthands", "desc": "Map Amp shorthand names such as g25p or cs45 to 9Router aliases in your local config."},
		}, "codeBlock": map[string]string{"language": "bash", "code": "export OPENAI_API_KEY=\"{{apiKey}}\"\nexport OPENAI_BASE_URL=\"{{baseUrl}}\"\namp --model \"{{model}}\""}},
		{"id": "qwen", "name": "Qwen Code", "description": "Alibaba Qwen Code CLI", "configType": "guide", "defaultCommand": "qwen", "guideSteps": []map[string]any{
			{"step": 1, "title": "Open Qwen Config", "desc": "Open ~/.qwen/settings.json"},
			{"step": 2, "title": "API Key", "type": "apiKeySelector"},
			{"step": 3, "title": "Base URL", "value": "{{baseUrl}}", "copyable": true},
			{"step": 4, "title": "Select Model", "type": "modelSelector"},
			{"step": 5, "title": "Configure OpenAI Provider", "desc": "Set 9Router as the OpenAI-compatible model provider."},
		}},
	}})
}
