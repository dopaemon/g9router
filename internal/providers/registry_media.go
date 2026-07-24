package providers

func init() {
	for id, descriptor := range map[string]Descriptor{
		"openrouter":        {ID: "openrouter", Alias: "openrouter", BaseURL: "https://openrouter.ai/api/v1/chat/completions", Format: "openai", Services: []string{"llm", "embedding", "imageToText"}},
		"minimax":           {ID: "minimax", Alias: "minimax", BaseURL: "https://api.minimax.io/v1/chat/completions", Format: "openai", Services: []string{"llm", "image", "tts", "imageToText"}, Models: []Model{{ID: "MiniMax-M3", Name: "MiniMax M3"}, {ID: "minimax-image-01", Name: "MiniMax Image 01", Kind: "image"}}},
		"recraft":           {ID: "recraft", Alias: "recraft", BaseURL: "https://external.api.recraft.ai/v1/images/generations", Format: "openai", Services: []string{"image"}, Models: []Model{{ID: "recraftv3", Name: "Recraft V3", Kind: "image"}}},
		"vercel-ai-gateway": {ID: "vercel-ai-gateway", Alias: "vercel-ai-gateway", BaseURL: "https://ai-gateway.vercel.sh/v1/chat/completions", Format: "openai", Services: []string{"llm", "embedding", "image", "imageToText", "webSearch"}},
		"xai":               {ID: "xai", Alias: "xai", BaseURL: "https://api.x.ai/v1/chat/completions", Format: "openai", Services: []string{"llm", "image", "video", "imageToText", "webSearch"}, Models: []Model{{ID: "grok-4", Name: "Grok 4"}, {ID: "grok-2-image-1212", Name: "Grok 2 Image", Kind: "image"}, {ID: "grok-imagine-video", Name: "Grok Imagine Video", Kind: "video"}}},
		"groq":              {ID: "groq", Alias: "groq", BaseURL: "https://api.groq.com/openai/v1/chat/completions", Format: "openai", Services: []string{"llm", "imageToText", "stt"}, Models: []Model{{ID: "llama-3.3-70b-versatile", Name: "Llama 3.3 70B"}, {ID: "whisper-large-v3", Name: "Whisper Large v3", Kind: "stt"}}},
		"ollama":            {ID: "ollama", Alias: "ollama", BaseURL: "https://ollama.com/api/chat", Format: "ollama", Services: []string{"llm"}},
		"qwen":              {ID: "qwen", Alias: "qwen", BaseURL: "https://portal.qwen.ai/v1/chat/completions", Format: "openai", Services: []string{"llm", "imageToText"}, Models: []Model{{ID: "qwen3-coder-plus", Name: "Qwen3 Coder Plus"}, {ID: "qwen3-coder-flash", Name: "Qwen3 Coder Flash"}}},
		"kimi":              {ID: "kimi", Alias: "kimi", BaseURL: "https://api.kimi.com/coding/v1/chat/completions", Format: "openai", Services: []string{"llm", "webSearch"}, Models: []Model{{ID: "kimi-k2.6", Name: "Kimi K2.6"}, {ID: "kimi-k2.5", Name: "Kimi K2.5"}}},
		"qoder":             {ID: "qoder", Alias: "qoder", BaseURL: "https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation", Format: "qoder", Services: []string{"llm"}, Models: []Model{{ID: "qmodel_latest", Name: "Qoder Qwen 3.7 Max"}}},
		"tavily":            {ID: "tavily", Alias: "tavily", BaseURL: "https://api.tavily.com/search", Format: "tavily", Services: []string{"webSearch"}},
		"deepgram":          {ID: "deepgram", Alias: "deepgram", BaseURL: "https://api.deepgram.com/v1/listen", Format: "deepgram", Services: []string{"stt"}, Models: []Model{{ID: "nova-3", Name: "Nova 3", Kind: "stt"}, {ID: "nova-2", Name: "Nova 2", Kind: "stt"}}},
		"assemblyai":        {ID: "assemblyai", Alias: "assemblyai", BaseURL: "https://api.assemblyai.com/v2/transcript", Format: "assemblyai", Services: []string{"stt"}, Models: []Model{{ID: "universal-3-pro", Name: "Universal 3 Pro", Kind: "stt"}, {ID: "universal-2", Name: "Universal 2", Kind: "stt"}}},
		"nvidia":            {ID: "nvidia", Alias: "nvidia", BaseURL: "https://integrate.api.nvidia.com/v1/chat/completions", Format: "openai", Services: []string{"stt", "tts", "embedding"}, Models: []Model{{ID: "nvidia/parakeet-ctc-1.1b-asr", Name: "Parakeet CTC 1.1B", Kind: "stt"}, {ID: "fastpitch", Name: "FastPitch", Kind: "tts"}}},
		"huggingface":       {ID: "huggingface", Alias: "huggingface", BaseURL: "https://api-inference.huggingface.co/models", Format: "huggingface", Services: []string{"stt", "image"}},
		"stability-ai":      {ID: "stability-ai", Alias: "stability-ai", BaseURL: "https://api.stability.ai/v2beta/stable-image/generate", Format: "stability-ai", Services: []string{"image"}, Models: []Model{{ID: "stable-image-ultra", Name: "Stable Image Ultra", Kind: "image"}, {ID: "stable-image-core", Name: "Stable Image Core", Kind: "image"}}},
		"cloudflare-ai":     {ID: "cloudflare-ai", Alias: "cloudflare-ai", BaseURL: "https://api.cloudflare.com/client/v4/accounts", Format: "cloudflare-ai", Services: []string{"image"}, Models: []Model{{ID: "@cf/black-forest-labs/flux-2-dev", Name: "FLUX.2 Dev", Kind: "image"}, {ID: "@cf/black-forest-labs/flux-2-klein-4b", Name: "FLUX.2 Klein 4B", Kind: "image"}}},
		"fal-ai":            {ID: "fal-ai", Alias: "fal-ai", BaseURL: "https://queue.fal.run", Format: "fal-ai", Services: []string{"image"}, Models: []Model{{ID: "fal-ai/flux/schnell", Name: "FLUX Schnell", Kind: "image"}, {ID: "fal-ai/flux/dev", Name: "FLUX Dev", Kind: "image"}}},
		"black-forest-labs": {ID: "black-forest-labs", Alias: "black-forest-labs", BaseURL: "https://api.bfl.ai/v1", Format: "black-forest-labs", Services: []string{"image"}, Models: []Model{{ID: "flux-pro-1.1", Name: "FLUX Pro 1.1", Kind: "image"}}},
		"runwayml":          {ID: "runwayml", Alias: "runwayml", BaseURL: "https://api.dev.runwayml.com/v1", Format: "runwayml", Services: []string{"image", "video"}, Models: []Model{{ID: "gen4_image", Name: "Gen-4 Image", Kind: "image"}}},
		"sdwebui":           {ID: "sdwebui", Alias: "sdwebui", BaseURL: "http://localhost:7860/sdapi/v1/txt2img", Format: "sdwebui", Services: []string{"image"}},
		"nanobanana":        {ID: "nanobanana", Alias: "nanobanana", BaseURL: "https://api.nanobananaapi.ai/api/v1/nanobanana/generate", Format: "nanobanana", Services: []string{"image"}, Models: []Model{{ID: "nanobanana-flash", Name: "NanoBanana Flash", Kind: "image"}, {ID: "nanobanana-pro", Name: "NanoBanana Pro", Kind: "image"}}},
	} {
		if _, exists := Registry[id]; !exists {
			Registry[id] = descriptor
		}
	}
}
