# g9router

Go tracer-bullet port of 9Router's OpenAI-compatible proxy core.

## Run

```bash
G9ROUTER_UPSTREAM=https://api.openai.com/v1 G9ROUTER_API_KEY=sk-... go run .
```

Defaults: `:20128`, upstream `https://api.openai.com/v1`.
Routes: `/healthz`, `/v1/models`, `/v1/chat/completions`, `/api/providers`, `/`.

OAuth credentials are managed through `/api/oauth` (`GET`, `POST`, `PUT?id=...`) and persisted in `oauth.json`; secrets are omitted from API responses.

OAuth flows that require public-client configuration read credentials from the environment: `G9ROUTER_GEMINI_CLIENT_ID`, `G9ROUTER_GEMINI_CLIENT_SECRET`, `G9ROUTER_ANTIGRAVITY_CLIENT_ID`, `G9ROUTER_ANTIGRAVITY_CLIENT_SECRET`, and `G9ROUTER_GROK_CLIENT_ID`. Device-code flows expose `/api/oauth/<provider>/device-code` and `/api/oauth/<provider>/poll`.

## Docker

```bash
docker build -t g9router:dev .
docker run --rm -p 20128:20128 -e G9ROUTER_UPSTREAM=https://api.openai.com/v1 -e G9ROUTER_API_KEY=sk-... g9router:dev
```

Client `Authorization` overrides `G9ROUTER_API_KEY`. SSE responses stream through without buffering.

Provider records persist in `providers.json`; API keys are never returned by the list endpoint.

## Layout

`cmd/g9router` contains the executable. `internal/server`, `internal/providers`, and `internal/web` contain private application modules. `web` is reserved for future external assets.

Provider-specific translators cover OpenAI, Claude, Gemini, Vertex, Codex, Kiro, and Cursor. The server also includes provider fallback, quota tracking, OAuth persistence, SQLite storage, model aliases/custom models, MCP bridges, Headroom APIs, CLI settings APIs, and RTK compression.
