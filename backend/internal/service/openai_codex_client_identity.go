package service

const (
	// openAICodexClientVersion 是项目内置 Codex 客户端身份使用的统一版本。
	// GPT-5.6 三个模型要求至少 0.144.0，使用对应已发布版本 0.144.1。
	openAICodexClientVersion = "0.144.1"
	codexCLIUserAgent        = "codex_cli_rs/" + openAICodexClientVersion + " (Ubuntu 22.4.0; x86_64) xterm-256color"
	codexCLIVersion          = openAICodexClientVersion
	openAICodexProbeVersion  = openAICodexClientVersion

	// DefaultOpenAICodexUserAgent 是 OpenAI Codex 的默认 User-Agent，用于规避浏览器 UA 触发的 Cloudflare 质询。
	DefaultOpenAICodexUserAgent = "codex-tui/" + openAICodexClientVersion + " (Ubuntu 22.4.0; x86_64) xterm-256color (codex-tui; " + openAICodexClientVersion + ")"
)
