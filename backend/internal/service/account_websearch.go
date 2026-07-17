package service

import "log/slog"

// WebSearch 模拟三态常量。
const (
	WebSearchModeDefault  = "default"  // 跟随渠道配置
	WebSearchModeEnabled  = "enabled"  // 强制开启
	WebSearchModeDisabled = "disabled" // 强制关闭
)

// GetWebSearchEmulationMode 返回账号的 WebSearch 模拟模式。
// 三态：default（跟随渠道）/ enabled（强制开启）/ disabled（强制关闭）。
// 兼容旧 bool 值：true→enabled，false→default（并记录 debug 日志）。
//
// @return 当前账号的 WebSearch 模拟模式。
func (a *Account) GetWebSearchEmulationMode() string {
	if a == nil || (a.Platform != PlatformAnthropic && a.Platform != PlatformOpenAI) || a.Type != AccountTypeAPIKey || a.Extra == nil {
		return WebSearchModeDefault
	}
	raw := a.Extra[featureKeyWebSearchEmulation]
	// 兼容迁移前或历史写入的布尔值，避免存量账号行为变化。
	if b, ok := raw.(bool); ok {
		slog.Debug("legacy bool web_search_emulation value", "account_id", a.ID, "value", b)
		if b {
			return WebSearchModeEnabled
		}
		return WebSearchModeDefault
	}
	mode, ok := raw.(string)
	if !ok {
		return WebSearchModeDefault
	}
	switch mode {
	case WebSearchModeEnabled, WebSearchModeDisabled:
		return mode
	default:
		return WebSearchModeDefault
	}
}
