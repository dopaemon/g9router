package providers

type Descriptor struct {
	ID, Alias, BaseURL, Format string
	Headers                    map[string]string
	Services                   []string
	Models                     []Model
}
type Model struct {
	ID, Name, Kind string
	Params         []string
}

var Registry = map[string]Descriptor{
	"openai":    {ID: "openai", Alias: "openai", BaseURL: "https://api.openai.com/v1/chat/completions", Format: "openai", Services: []string{"llm", "embedding", "tts", "stt", "image", "imageToText", "webSearch"}, Models: []Model{{ID: "gpt-5.4", Name: "GPT-5.4"}, {ID: "gpt-5.4-mini", Name: "GPT-5.4 Mini"}, {ID: "gpt-4o", Name: "GPT-4o"}, {ID: "gpt-4o-mini", Name: "GPT-4o Mini"}, {ID: "text-embedding-3-large", Name: "Text Embedding 3 Large", Kind: "embedding"}, {ID: "tts-1", Name: "TTS-1", Kind: "tts"}, {ID: "whisper-1", Name: "Whisper 1", Kind: "stt"}, {ID: "gpt-image-1", Name: "GPT Image 1", Kind: "image"}}},
	"anthropic": {ID: "anthropic", Alias: "anthropic", BaseURL: "https://api.anthropic.com/v1/messages", Format: "claude", Headers: map[string]string{"anthropic-version": "2023-06-01", "Anthropic-Beta": "claude-code-20250219,interleaved-thinking-2025-05-14"}, Services: []string{"llm", "imageToText"}, Models: []Model{{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4"}, {ID: "claude-opus-4-20250514", Name: "Claude Opus 4"}, {ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet"}}},
	"gemini":    {ID: "gemini", Alias: "gemini", BaseURL: "https://generativelanguage.googleapis.com/v1beta", Format: "gemini", Services: []string{"llm", "embedding", "image", "imageToText", "tts", "stt"}, Models: []Model{{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro"}, {ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash"}, {ID: "gemini-2.5-flash-lite", Name: "Gemini 2.5 Flash Lite"}, {ID: "gemini-embedding-001", Name: "Gemini Embedding 001", Kind: "embedding"}, {ID: "gemini-2.5-flash-image", Name: "Gemini 2.5 Flash Image", Kind: "image"}}},
}

func Lookup(id string) (Descriptor, bool) { descriptor, ok := Registry[id]; return descriptor, ok }
