package providers

func init() {
	for id, descriptor := range map[string]Descriptor{
		"deepseek":       {ID: "deepseek", Alias: "deepseek", BaseURL: "https://api.deepseek.com/chat/completions", Format: "openai", Services: []string{"llm"}},
		"mistral":        {ID: "mistral", Alias: "mistral", BaseURL: "https://api.mistral.ai/v1/chat/completions", Format: "openai", Services: []string{"llm", "embedding"}},
		"cohere":         {ID: "cohere", Alias: "cohere", BaseURL: "https://api.cohere.com/v2/chat", Format: "openai", Services: []string{"llm", "embedding"}},
		"cerebras":       {ID: "cerebras", Alias: "cerebras", BaseURL: "https://api.cerebras.ai/v1/chat/completions", Format: "openai", Services: []string{"llm"}},
		"together":       {ID: "together", Alias: "together", BaseURL: "https://api.together.xyz/v1/chat/completions", Format: "openai", Services: []string{"llm", "imageToText"}},
		"fireworks":      {ID: "fireworks", Alias: "fireworks", BaseURL: "https://api.fireworks.ai/inference/v1/chat/completions", Format: "openai", Services: []string{"llm", "imageToText"}},
		"siliconflow":    {ID: "siliconflow", Alias: "siliconflow", BaseURL: "https://api.siliconflow.cn/v1/chat/completions", Format: "openai", Services: []string{"llm", "embedding", "image"}},
		"nebius":         {ID: "nebius", Alias: "nebius", BaseURL: "https://api.studio.nebius.ai/v1/chat/completions", Format: "openai", Services: []string{"llm"}},
		"hyperbolic":     {ID: "hyperbolic", Alias: "hyperbolic", BaseURL: "https://api.hyperbolic.xyz/v1/chat/completions", Format: "openai", Services: []string{"llm", "image"}},
		"chutes":         {ID: "chutes", Alias: "chutes", BaseURL: "https://llm.chutes.ai/v1/chat/completions", Format: "openai", Services: []string{"llm"}},
		"venice":         {ID: "venice", Alias: "venice", BaseURL: "https://api.venice.ai/api/v1/chat/completions", Format: "openai", Services: []string{"llm", "image"}},
		"glm":            {ID: "glm", Alias: "glm", BaseURL: "https://open.bigmodel.cn/api/paas/v4/chat/completions", Format: "openai", Services: []string{"llm", "imageToText"}},
		"glm-cn":         {ID: "glm-cn", Alias: "glm-cn", BaseURL: "https://open.bigmodel.cn/api/paas/v4/chat/completions", Format: "openai", Services: []string{"llm", "imageToText"}},
		"byteplus":       {ID: "byteplus", Alias: "byteplus", BaseURL: "https://ark.cn-beijing.volces.com/api/v3/chat/completions", Format: "openai", Services: []string{"llm", "image"}},
		"volcengine-ark": {ID: "volcengine-ark", Alias: "volcengine-ark", BaseURL: "https://ark.cn-beijing.volces.com/api/v3/chat/completions", Format: "openai", Services: []string{"llm", "image"}},
		"alicode":        {ID: "alicode", Alias: "alicode", BaseURL: "https://coding.dashscope.aliyuncs.com/v1/chat/completions", Format: "openai", Services: []string{"llm"}},
		"alicode-intl":   {ID: "alicode-intl", Alias: "alicode-intl", BaseURL: "https://coding-intl.dashscope.aliyuncs.com/v1/chat/completions", Format: "openai", Services: []string{"llm"}},
		"alims-intl":     {ID: "alims-intl", Alias: "alims-intl", BaseURL: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1/chat/completions", Format: "openai", Services: []string{"llm", "image"}},
		"jina-ai":        {ID: "jina-ai", Alias: "jina", BaseURL: "https://api.jina.ai/v1/chat/completions", Format: "openai", Services: []string{"llm", "embedding"}},
		"opencode":       {ID: "opencode", Alias: "oc", BaseURL: "https://api.opencode.ai/v1/chat/completions", Format: "openai", Services: []string{"llm"}},
		"opencode-go":    {ID: "opencode-go", Alias: "opencode-go", BaseURL: "https://opencode.ai/zen/v1/chat/completions", Format: "openai", Services: []string{"llm"}},
	} {
		if _, exists := Registry[id]; !exists {
			Registry[id] = descriptor
		}
	}
}
