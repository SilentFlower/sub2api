package service

import "github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"

// IsOpenAIJSONSchemaToJSONObjectEnabled 返回账号是否启用 JSON Schema 兼容模式。
//
// @return 仅 OpenAI APIKey 账号且 extra 中严格配置为 true 时返回 true。
func (a *Account) IsOpenAIJSONSchemaToJSONObjectEnabled() bool {
	if a == nil || !a.IsOpenAIApiKey() {
		return false
	}
	return openai_compat.JSONSchemaToJSONObjectEnabled(a.Extra)
}
