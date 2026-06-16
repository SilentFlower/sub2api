package openai

// MatchCustomUserAgentPrefixes 判断 User-Agent 是否命中任一自定义前缀规则。
//
// 自定义规则面向账号级 codex_cli_only 放行配置：每个 pattern 都按前缀匹配，
// 其中 `*` 表示任意字符序列。空白 pattern 永不匹配；多个 pattern 任一命中即放行。
func MatchCustomUserAgentPrefixes(userAgent string, patterns []string) bool {
	ua := normalizeCodexClientHeader(userAgent)
	if ua == "" {
		return false
	}
	for _, pattern := range patterns {
		normalizedPattern := normalizeCodexClientHeader(pattern)
		if normalizedPattern == "" {
			continue
		}
		if matchUserAgentPrefixPattern(ua, normalizedPattern) {
			return true
		}
	}
	return false
}

func matchUserAgentPrefixPattern(userAgent, pattern string) bool {
	if pattern == "*" {
		return true
	}
	return matchGlobPrefix(userAgent, pattern)
}

func matchGlobPrefix(value, pattern string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	parts := splitGlobPattern(pattern)
	if len(parts) == 1 {
		return hasPrefix(value, parts[0])
	}

	pos := 0
	if parts[0] != "" {
		if !hasPrefix(value, parts[0]) {
			return false
		}
		pos = len(parts[0])
	}
	for i := 1; i < len(parts); i++ {
		part := parts[i]
		if part == "" {
			continue
		}
		next := indexFrom(value, part, pos)
		if next < 0 {
			return false
		}
		pos = next + len(part)
	}
	return true
}

func splitGlobPattern(pattern string) []string {
	parts := []string{""}
	for _, r := range pattern {
		if r == '*' {
			parts = append(parts, "")
			continue
		}
		parts[len(parts)-1] += string(r)
	}
	return parts
}

func hasPrefix(value, prefix string) bool {
	if len(prefix) > len(value) {
		return false
	}
	return value[:len(prefix)] == prefix
}

func indexFrom(value, needle string, start int) int {
	if needle == "" {
		return start
	}
	if start < 0 {
		start = 0
	}
	if start > len(value) || len(needle) > len(value)-start {
		return -1
	}
	for i := start; i <= len(value)-len(needle); i++ {
		if value[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
