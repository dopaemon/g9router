# g9router

Go port of 9Router's OpenAI-compatible gateway, including provider routing, OAuth, translations, media APIs, management APIs, and embedded dashboard.

## Run

```bash
G9ROUTER_UPSTREAM=https://api.openai.com/v1 G9ROUTER_API_KEY=sk-... go run .
```

Defaults: `:20128`, upstream `https://api.openai.com/v1`.
Routes include `/healthz`, `/v1/models`, `/v1/chat/completions`, `/v1/responses`, `/v1/messages`, `/v1/embeddings`, `/v1/images/generations`, `/v1/audio/*`, `/v1/search`, `/v1/web/fetch`, `/api/providers`, `/api/oauth`, and `/dashboard`.

OAuth credentials are managed through `/api/oauth` (`GET`, `POST`, `PUT?id=...`) and persisted in `oauth.json`; secrets are omitted from API responses.

OAuth flows that require public-client configuration read credentials from the environment: `G9ROUTER_GEMINI_CLIENT_ID`, `G9ROUTER_GEMINI_CLIENT_SECRET`, `G9ROUTER_ANTIGRAVITY_CLIENT_ID`, `G9ROUTER_ANTIGRAVITY_CLIENT_SECRET`, `G9ROUTER_GROK_CLIENT_ID`, `G9ROUTER_IFLOW_CLIENT_ID`, and `G9ROUTER_IFLOW_CLIENT_SECRET`. Device-code flows expose `/api/oauth/<provider>/device-code` and `/api/oauth/<provider>/poll`.

## Docker

```bash
docker build -t g9router:dev .
docker run --rm -p 20128:20128 -e G9ROUTER_UPSTREAM=https://api.openai.com/v1 -e G9ROUTER_API_KEY=sk-... g9router:dev
```

Client `Authorization` overrides `G9ROUTER_API_KEY`. SSE responses stream through without buffering.

Provider records persist in `providers.json`; API keys are never returned by the list endpoint.

## Layout

`cmd/g9router` contains the executable. `internal/server` owns HTTP routing and provider dispatch; `internal/providers` owns the registry and persistence; `internal/translator` owns wire-format conversion; `internal/oauth`, `internal/keys`, and `internal/settings` own persisted management state; `internal/web` embeds the dashboard. The top-level `web` directory is reserved for external assets.

Provider-specific paths cover OpenAI, Claude, Gemini, Gemini CLI, Vertex, Antigravity, Codex, Grok CLI, Kiro, Cursor, Qoder, Qwen, iFlow, Kimchi, MiMo, Perplexity Web, OpenCode, CommandCode, and media providers. The server also includes provider fallback, quota tracking, OAuth persistence, SQLite storage, model aliases/custom models, MCP bridges, Headroom APIs, CLI settings APIs, tunnels, proxy pools, and RTK compression.
