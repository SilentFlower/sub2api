package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// codexBaseInstructionsForModel 为模型目录和请求补全选择同一份模型专用提示词。
// @param model 原始模型名称，可包含现有规则支持的 provider 路径和后缀别名。
// @return 对应的内嵌提示词，旧型号和未知型号保持原有选择规则。
func codexBaseInstructionsForModel(model string) string {
	// 只归一化新模板对应的型号，避免改变旧版本专用模板和 codex 模型的优先级。
	if !strings.Contains(strings.ToLower(model), "codex") {
		switch normalized := normalizeKnownOpenAICodexModel(model); normalized {
		case "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna":
			model = normalized
		}
	}
	return openai.CodexBaseInstructionsForModel(model)
}
