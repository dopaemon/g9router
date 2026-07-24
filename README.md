# g9router

Go tracer-bullet port of 9Router's OpenAI-compatible proxy core.

## Run

```bash
G9ROUTER_UPSTREAM=https://api.openai.com/v1 G9ROUTER_API_KEY=sk-... go run .
```

Defaults: `:20128`, upstream `https://api.openai.com/v1`.
Routes: `/healthz`, `/v1/models`, `/v1/chat/completions`, `/api/providers`, `/`.

Client `Authorization` overrides `G9ROUTER_API_KEY`. SSE responses stream through without buffering.

Provider records persist in `providers.json`; API keys are never returned by the list endpoint.

## Layout

`cmd/g9router` contains the executable. `internal/server`, `internal/providers`, and `internal/web` contain private application modules. `web` is reserved for future external assets.

Provider-specific translators, automatic fallback, quota tracking, OAuth, SQLite, and RTK remain future parity work.
