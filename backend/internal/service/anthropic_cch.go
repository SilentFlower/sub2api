package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/tidwall/gjson"
)

const (
	anthropicCCHModeFixed           = "fixed"
	anthropicCCHModeUserAgent       = "user_agent"
	anthropicCCHDefaultFixedVersion = "2.1.90"
	anthropicCCHPlaceholder         = "00000"
	anthropicCCHSeed                = uint64(0x6E52736AC806831E)
	anthropicCCHMask                = uint64(0xFFFFF)
)

// applyAnthropicCCHOverride 按账号配置强制覆盖 billing header，并基于最终出站 body 重算 cch。
// 仅对 Anthropic OAuth/SetupToken 且显式启用的账号生效。
func applyAnthropicCCHOverride(req *http.Request, body []byte, account *Account) ([]byte, bool) {
	if req == nil || account == nil || !account.IsAnthropicOAuthOrSetupToken() || !account.IsAnthropicCCHEnabled() {
		return body, false
	}

	if account.ShouldRewriteAnthropicCCHUserAgent() {
		rewrittenUA := rewriteAnthropicCCHUserAgent(getHeaderRaw(req.Header, "User-Agent"), account.GetAnthropicCCHFixedVersion())
		if rewrittenUA != "" {
			setHeaderRaw(req.Header, "User-Agent", rewrittenUA)
		}
	}

	version := resolveAnthropicCCHVersion(req, account)
	placeholderBody, ok := overwriteAnthropicBillingHeaderInBody(body, version, anthropicCCHPlaceholder)
	if !ok {
		return body, false
	}

	cch := computeAnthropicCCH(placeholderBody)
	finalBody, ok := overwriteAnthropicBillingHeaderInBody(placeholderBody, version, cch)
	if !ok {
		return body, false
	}

	setHeaderRaw(req.Header, "x-anthropic-billing-header", formatAnthropicBillingHeaderValue(version, cch))
	req.ContentLength = int64(len(finalBody))
	req.Body = io.NopCloser(bytes.NewReader(finalBody))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(finalBody)), nil
	}

	return finalBody, true
}

func resolveAnthropicCCHVersion(req *http.Request, account *Account) string {
	if account == nil {
		return anthropicCCHDefaultFixedVersion
	}
	if account.GetAnthropicCCHMode() == anthropicCCHModeUserAgent {
		if version := ExtractCLIVersion(getHeaderRaw(req.Header, "User-Agent")); version != "" {
			return version
		}
	}
	return account.GetAnthropicCCHFixedVersion()
}

func rewriteAnthropicCCHUserAgent(userAgent, version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		version = anthropicCCHDefaultFixedVersion
	}
	if strings.TrimSpace(userAgent) == "" {
		return fmt.Sprintf("claude-cli/%s (external, cli)", version)
	}
	if claudeCodeUAVersionPattern.MatchString(userAgent) {
		return claudeCodeUAVersionPattern.ReplaceAllString(userAgent, "claude-cli/"+version)
	}
	return fmt.Sprintf("claude-cli/%s (external, cli)", version)
}

func computeAnthropicCCH(body []byte) string {
	h := xxhash.NewWithSeed(anthropicCCHSeed)
	_, _ = h.Write(body)
	return fmt.Sprintf("%05x", h.Sum64()&anthropicCCHMask)
}

func formatAnthropicBillingHeaderValue(version, cch string) string {
	return fmt.Sprintf("cc_version=%s; cc_entrypoint=cli; cch=%s;", version, cch)
}

func formatAnthropicBillingHeaderSystemText(version, cch string) string {
	return "x-anthropic-billing-header: " + formatAnthropicBillingHeaderValue(version, cch)
}

func overwriteAnthropicBillingHeaderInBody(body []byte, version, cch string) ([]byte, bool) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, false
	}

	billingText := formatAnthropicBillingHeaderSystemText(version, cch)
	system := gjson.GetBytes(body, "system")
	if !system.Exists() || system.Type == gjson.Null {
		block, err := marshalAnthropicSystemTextBlock(billingText, false)
		if err != nil {
			return body, false
		}
		return setJSONRawBytes(body, "system", buildJSONArrayRaw([][]byte{block}))
	}

	switch {
	case system.Type == gjson.String:
		cleaned := stripAnthropicBillingHeaderText(system.String())
		if cleaned == "" {
			return setJSONValueBytes(body, "system", billingText)
		}
		return setJSONValueBytes(body, "system", billingText+"\n"+cleaned)
	case system.IsArray():
		items := make([][]byte, 0, 4)
		block, err := marshalAnthropicSystemTextBlock(billingText, false)
		if err != nil {
			return body, false
		}
		items = append(items, block)

		system.ForEach(func(_, item gjson.Result) bool {
			raw, keep := sanitizeAnthropicSystemItem(body, item)
			if keep {
				items = append(items, raw)
			}
			return true
		})

		return setJSONRawBytes(body, "system", buildJSONArrayRaw(items))
	default:
		block, err := marshalAnthropicSystemTextBlock(billingText, false)
		if err != nil {
			return body, false
		}
		return setJSONRawBytes(body, "system", buildJSONArrayRaw([][]byte{block}))
	}
}

func sanitizeAnthropicSystemItem(body []byte, item gjson.Result) ([]byte, bool) {
	switch item.Type {
	case gjson.String:
		cleaned := stripAnthropicBillingHeaderText(item.String())
		if cleaned == "" {
			return nil, false
		}
		if cleaned == item.String() {
			return sliceRawFromBody(body, item), true
		}
		return []byte(strconv.Quote(cleaned)), true
	default:
		text := item.Get("text")
		if text.Exists() && text.Type == gjson.String {
			cleaned := stripAnthropicBillingHeaderText(text.String())
			if cleaned == "" {
				return nil, false
			}
			if cleaned != text.String() {
				raw := sliceRawFromBody(body, item)
				if next, ok := setJSONValueBytes(raw, "text", cleaned); ok {
					return next, true
				}
			}
		}
		return sliceRawFromBody(body, item), true
	}
}

func stripAnthropicBillingHeaderText(text string) string {
	if text == "" {
		return ""
	}

	lines := strings.Split(text, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "x-anthropic-billing-header:") {
			continue
		}
		filtered = append(filtered, line)
	}

	return strings.Trim(strings.Join(filtered, "\n"), "\r\n")
}
