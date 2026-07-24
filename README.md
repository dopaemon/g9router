# g9router

Go tracer-bullet port of 9Router's OpenAI-compatible proxy core.

## Run

```bash
G9ROUTER_UPSTREAM=https://api.openai.com/v1 G9ROUTER_API_KEY=sk-... go run .
```

Defaults: `:20128`, upstream `https://api.openai.com/v1`.
Routes: `/healthz`, `/v1/models`, `/v1/chat/completions`.

Client `Authorization` overrides `G9ROUTER_API_KEY`. SSE responses stream through without buffering.

Provider adapters, fallback, quota, OAuth, database, dashboard, and RTK remain unported.
