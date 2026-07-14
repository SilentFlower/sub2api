package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const anySearchProviderName = ProviderTypeAnySearch

var anySearchEndpoint = "https://api.anysearch.com/mcp"

// AnySearchProvider 通过 AnySearch-compatible MCP JSON-RPC 接口执行网页搜索。
type AnySearchProvider struct {
	apiKey     string
	httpClient *http.Client
}

// NewAnySearchProvider 创建 AnySearch-compatible 搜索 provider。
//
// @param apiKey 可选 Bearer API Key。
// @param httpClient 已配置代理与超时的 HTTP client；nil 时使用默认 client。
// @return 可执行搜索的 provider。
func NewAnySearchProvider(apiKey string, httpClient *http.Client) *AnySearchProvider {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &AnySearchProvider{apiKey: strings.TrimSpace(apiKey), httpClient: httpClient}
}

// Name 返回 AnySearch provider 标识。
//
// @return 固定值 anysearch。
func (p *AnySearchProvider) Name() string { return anySearchProviderName }

// Search 调用 MCP tools/call 并把多种兼容响应归一化为统一搜索结果。
//
// @param ctx 请求上下文。
// @param req 搜索请求。
// @return 归一化搜索响应或脱敏后的调用错误。
func (p *AnySearchProvider) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	maxResults := req.MaxResults
	if maxResults <= 0 {
		maxResults = defaultMaxResults
	}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "search",
			"arguments": map[string]any{
				"query":       req.Query,
				"max_results": maxResults,
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("anysearch: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anySearchEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anysearch: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anysearch: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("anysearch: read body: %w", err)
	}
	if len(responseBody) > maxResponseSize {
		return nil, fmt.Errorf("anysearch: response body exceeds %d bytes", maxResponseSize)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("anysearch: status %d: %s", resp.StatusCode, redactAnySearchError(responseBody, p.apiKey, req.Query))
	}

	var rpc struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(responseBody, &rpc); err != nil {
		return nil, fmt.Errorf("anysearch: decode response: %w", err)
	}
	if rpc.Error != nil {
		message := redactAnySearchError([]byte(rpc.Error.Message), p.apiKey, req.Query)
		return nil, fmt.Errorf("anysearch: rpc error %d: %s", rpc.Error.Code, message)
	}
	if len(rpc.Result) == 0 || string(rpc.Result) == "null" {
		return nil, fmt.Errorf("anysearch: response missing result")
	}

	results := normalizeAnySearchResults(rpc.Result)
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return &SearchResponse{Results: results, Query: req.Query}, nil
}

func redactAnySearchError(body []byte, secrets ...string) string {
	// 必须先在完整响应中脱敏再截断；否则长密钥被截断后无法完整匹配，前缀会进入错误信息。
	text := string(body)
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	return truncateBody([]byte(text))
}

func normalizeAnySearchResults(raw json.RawMessage) []SearchResult {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	results := collectAnySearchResults(value)
	seen := make(map[string]struct{}, len(results))
	out := make([]SearchResult, 0, len(results))
	for _, result := range results {
		result.URL = strings.TrimSpace(result.URL)
		result.Title = strings.TrimSpace(result.Title)
		result.Snippet = strings.TrimSpace(result.Snippet)
		if result.URL == "" && result.Title == "" && result.Snippet == "" {
			continue
		}
		key := result.URL + "\x00" + result.Title + "\x00" + result.Snippet
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, result)
	}
	return out
}

func collectAnySearchResults(value any) []SearchResult {
	switch typed := value.(type) {
	case []any:
		var results []SearchResult
		for _, item := range typed {
			results = append(results, collectAnySearchResults(item)...)
		}
		return results
	case map[string]any:
		if result, ok := anySearchResultFromMap(typed); ok {
			return []SearchResult{result}
		}
		for _, key := range []string{"structuredContent", "structured_content", "results", "items", "data", "list", "result"} {
			if nested, exists := typed[key]; exists {
				if results := collectAnySearchResults(nested); len(results) > 0 {
					return results
				}
			}
		}
		if content, ok := typed["content"].([]any); ok {
			var fallback []SearchResult
			for _, item := range content {
				block, ok := item.(map[string]any)
				if !ok || stringValue(block, "type") != "text" {
					continue
				}
				text := strings.TrimSpace(stringValue(block, "text"))
				if text == "" {
					continue
				}
				var decoded any
				if json.Unmarshal([]byte(text), &decoded) == nil {
					if results := collectAnySearchResults(decoded); len(results) > 0 {
						return results
					}
				}
				fallback = append(fallback, SearchResult{Title: "AnySearch", Snippet: text})
			}
			return fallback
		}
	}
	return nil
}

func anySearchResultFromMap(value map[string]any) (SearchResult, bool) {
	url := firstStringValue(value, "url", "link", "href")
	title := firstStringValue(value, "title", "name")
	snippet := firstStringValue(value, "snippet", "content", "description", "text", "page_content")
	if url == "" && title == "" {
		return SearchResult{}, false
	}
	if title == "" {
		title = url
	}
	return SearchResult{URL: url, Title: title, Snippet: snippet}, true
}

func firstStringValue(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if result := stringValue(value, key); result != "" {
			return result
		}
	}
	return ""
}

func stringValue(value map[string]any, key string) string {
	text, _ := value[key].(string)
	return strings.TrimSpace(text)
}
