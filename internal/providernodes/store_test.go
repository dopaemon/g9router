package providernodes

import "testing"

func TestCreateSanitizesTypedEndpoint(t *testing.T) {
	store := New(t.TempDir() + "/nodes.json")
	item, err := store.Create(Node{Name: "Embeddings", Prefix: "emb", Type: "custom-embedding", BaseURL: "https://example.test/embeddings/"})
	if err != nil || item.BaseURL != "https://example.test" {
		t.Fatal(item, err)
	}
}

func TestCreateRejectsInvalidOpenAIType(t *testing.T) {
	store := New(t.TempDir() + "/nodes.json")
	if _, err := store.Create(Node{Name: "Chat", Prefix: "x", Type: "openai-compatible", APIType: "invalid", BaseURL: "https://example.test"}); err == nil {
		t.Fatal("expected validation error")
	}
}
