package service

import "strings"

const (
	codexCLIOnlyCustomUserAgentPrefixMaxCount  = 32
	codexCLIOnlyCustomUserAgentPrefixMaxLength = 256
)

// GetCodexCLIOnlyCustomUserAgentPrefixes 返回 codex_cli_only 之上额外放行的自定义 User-Agent 前缀规则。
// 仅 OpenAI OAuth 账号生效；缺失或类型不符时返回空。规则中的 `*` 由 openai 包按通配符处理。
//
// @return 清理、去重并限制数量后的 User-Agent 前缀规则。
func (a *Account) GetCodexCLIOnlyCustomUserAgentPrefixes() []string {
	if a == nil || !a.IsOpenAIOAuth() || a.Extra == nil {
		return nil
	}
	raw, ok := a.Extra["codex_cli_only_custom_user_agent_prefixes"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return compactCodexCustomUserAgentPrefixes(v)
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return compactCodexCustomUserAgentPrefixes(result)
	}
	return nil
}

func compactCodexCustomUserAgentPrefixes(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		pattern := strings.TrimSpace(value)
		if pattern == "" || len(pattern) > codexCLIOnlyCustomUserAgentPrefixMaxLength {
			continue
		}
		key := strings.ToLower(pattern)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, pattern)
		if len(result) >= codexCLIOnlyCustomUserAgentPrefixMaxCount {
			break
		}
	}
	return result
}
