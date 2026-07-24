package server

import "testing"

func TestPerplexitySessionRoundTrip(t *testing.T) {
	perplexitySessions.Lock()
	perplexitySessions.values = map[string]perplexitySession{}
	perplexitySessions.Unlock()
	perplexitySessionStore(nil, "old", "answer", "backend-1")
	lookup := []any{map[string]string{"role": "user", "content": "old"}, map[string]string{"role": "assistant", "content": "answer"}}
	if got := perplexitySessionLookup(lookup); got != "backend-1" {
		t.Fatalf("backend = %q", got)
	}
}
