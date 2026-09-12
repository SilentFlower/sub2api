package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const (
	// openAIAlphaSearchEmulationMaxQueries 与 Codex web.run 描述一致：单次调用最多 4 条查询。
	openAIAlphaSearchEmulationMaxQueries = 4
	// openAIAlphaSearchEmulationEndpoint 是本地模拟结果记录到用量的上游端点标记。
	openAIAlphaSearchEmulationEndpoint = "/v1/alpha/search"
)

// openAIAlphaSearchUnsupportedCommands 是本地模拟不执行、只在 output 中给出说明的 web.run 命令。
var openAIAlphaSearchUnsupportedCommands = []string{"open", "click", "find", "screenshot", "finance", "weather", "sports", "time"}

// openAIAlphaSearchEmulationQuery 是一条待执行的文本查询及其查询级允许域名。
type openAIAlphaSearchEmulationQuery struct {
	Query          string
	AllowedDomains []string
}

// openAIAlphaSearchEmulationPlan 是从 alpha 请求体解析出的本地模拟执行计划。
type openAIAlphaSearchEmulationPlan struct {
	Queries        []openAIAlphaSearchEmulationQuery
	AllowedDomains []string
	BlockedDomains []string
	MaxResults     int
	MaxOutputChars int
	Unsupported    []string
}

// openAIAlphaSearchEmulationBlock 是一条查询的过滤去重后结果。
type openAIAlphaSearchEmulationBlock struct {
	Query   string
	Results []websearch.SearchResult
}

// alphaSearchEmulationEligible 报告账号是否具备本地模拟搜索资格，判定与 Codex
// Web Search 桥接一致：账号/渠道开启 Web Search Emulation、系统设置开启且存在可用供应商。
//
// @param ctx 请求上下文。
// @param c Gin 请求上下文，用于读取分组渠道。
// @param account 当前账号。
// @return 具备资格返回 true。
func (s *OpenAIGatewayService) alphaSearchEmulationEligible(ctx context.Context, c *gin.Context, account *Account) bool {
	if s == nil || account == nil || !s.isOpenAIWebSearchEmulationEnabled(ctx, c, account) {
		return false
	}
	if s.settingService == nil || !s.settingService.IsWebSearchEmulationEnabled(ctx) {
		return false
	}
	manager := getWebSearchManager()
	return manager != nil && manager.HasAvailableProvider(ctx, resolveAccountProxyURL(account))
}

// parseOpenAIAlphaSearchEmulationPlan 从 alpha 请求体提取本地模拟需要的查询、过滤与限制。
// 只读取必要字段，不把 alpha 请求绑定到本地 DTO，避免丢失仍在演进的未知字段。
//
// @param alphaBody 原始 alpha/search 请求体。
// @return 解析出的执行计划。
func parseOpenAIAlphaSearchEmulationPlan(alphaBody []byte) openAIAlphaSearchEmulationPlan {
	plan := openAIAlphaSearchEmulationPlan{MaxResults: webSearchDefaultMaxResults}
	switch strings.TrimSpace(gjson.GetBytes(alphaBody, "settings.search_context_size").String()) {
	case "low":
		plan.MaxResults = 3
	case "high":
		plan.MaxResults = 10
	}
	plan.AllowedDomains = openAIAlphaSearchStringSlice(gjson.GetBytes(alphaBody, "settings.filters.allowed_domains"))
	plan.BlockedDomains = openAIAlphaSearchStringSlice(gjson.GetBytes(alphaBody, "settings.filters.blocked_domains"))
	if tokens := gjson.GetBytes(alphaBody, "max_output_tokens"); tokens.Exists() && tokens.Int() > 0 {
		plan.MaxOutputChars = int(tokens.Int()) * 4
	}

	seen := make(map[string]struct{})
	for _, path := range []string{"commands.search_query", "commands.image_query"} {
		for _, item := range gjson.GetBytes(alphaBody, path).Array() {
			if len(plan.Queries) >= openAIAlphaSearchEmulationMaxQueries {
				break
			}
			query := strings.TrimSpace(item.Get("q").String())
			if query == "" {
				continue
			}
			key := strings.ToLower(query)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			plan.Queries = append(plan.Queries, openAIAlphaSearchEmulationQuery{
				Query:          query,
				AllowedDomains: openAIAlphaSearchStringSlice(item.Get("domains")),
			})
		}
	}
	for _, name := range openAIAlphaSearchUnsupportedCommands {
		if commands := gjson.GetBytes(alphaBody, "commands."+name); commands.IsArray() && len(commands.Array()) > 0 {
			plan.Unsupported = append(plan.Unsupported, name)
		}
	}
	return plan
}

// openAIAlphaSearchStringSlice 把 gjson 数组转换为去空白的字符串切片。
//
// @param value gjson 结果，非数组时返回 nil。
// @return 非空字符串切片。
func openAIAlphaSearchStringSlice(value gjson.Result) []string {
	if !value.IsArray() {
		return nil
	}
	items := value.Array()
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text := strings.TrimSpace(item.String()); text != "" {
			out = append(out, strings.ToLower(text))
		}
	}
	return out
}

// emulateOpenAIAlphaSearch 用本地 Web Search Emulation 供应商执行 alpha 请求中的
// search_query，并按 Codex SearchResponse 形态写回 {"output","results"}。
//
// 只含不支持命令或零结果时写回 200 说明文本但不计费；所有查询都失败时写回 502 并
// 返回错误；至少一条查询成功且有结果时按 WebSearchCalls=1 计费。
//
// @param ctx 请求上下文。
// @param c Gin 请求上下文，响应直接写入。
// @param account 当前账号，用于代理与执行器。
// @param alphaBody 原始 alpha/search 请求体。
// @param requestedModel 客户端请求的模型名。
// @param upstreamModel 映射后的上游模型名。
// @return 有真实结果时返回 WebSearchCalls=1 的结果；不计费响应返回 (nil, nil)；供应商全部失败返回错误。
func (s *OpenAIGatewayService) emulateOpenAIAlphaSearch(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	alphaBody []byte,
	requestedModel string,
	upstreamModel string,
) (*OpenAIForwardResult, error) {
	start := time.Now()
	plan := parseOpenAIAlphaSearchEmulationPlan(alphaBody)
	SetActualOpenAIUpstreamEndpoint(c, openAIAlphaSearchEmulationEndpoint)
	if len(plan.Queries) == 0 {
		output, _ := buildOpenAIAlphaSearchEmulationOutput(nil, plan.Unsupported, plan.MaxOutputChars)
		return nil, writeOpenAIAlphaSearchEmulationResponse(c, output, nil)
	}

	executor := doWebSearchWithMaxResults
	if s.openAIWebSearchExecutor != nil {
		executor = s.openAIWebSearchExecutor
	}
	blocks := make([]openAIAlphaSearchEmulationBlock, 0, len(plan.Queries))
	providers := make(map[string]bool)
	seenURLs := make(map[string]struct{})
	failures := 0
	for _, query := range plan.Queries {
		resp, provider, err := executor(ctx, account, query.Query, plan.MaxResults)
		if err != nil || resp == nil {
			failures++
			logger.L().Warn("openai alpha search emulation: query failed",
				zap.Int64("account_id", account.ID),
				zap.Error(err),
			)
			continue
		}
		if provider != "" {
			providers[provider] = true
		}
		allowed := append(append([]string{}, plan.AllowedDomains...), query.AllowedDomains...)
		filtered := filterOpenAIResponsesSearchResults(resp.Results, allowed, plan.BlockedDomains)
		deduped := make([]websearch.SearchResult, 0, len(filtered))
		for _, result := range filtered {
			key := strings.TrimSpace(result.URL)
			if key == "" {
				continue
			}
			if _, dup := seenURLs[key]; dup {
				continue
			}
			seenURLs[key] = struct{}{}
			deduped = append(deduped, result)
		}
		blocks = append(blocks, openAIAlphaSearchEmulationBlock{Query: query.Query, Results: deduped})
	}
	if failures == len(plan.Queries) {
		writeOpenAIAlphaSearchFailed(c, "All configured web search providers failed")
		return nil, errors.New("alpha search emulation: all web search providers failed")
	}

	output, results := buildOpenAIAlphaSearchEmulationOutput(blocks, plan.Unsupported, plan.MaxOutputChars)
	if err := writeOpenAIAlphaSearchEmulationResponse(c, output, results); err != nil {
		return nil, err
	}
	logger.L().Info("openai alpha search emulation completed",
		zap.Int64("account_id", account.ID),
		zap.String("model", upstreamModel),
		zap.Int("queries", len(plan.Queries)),
		zap.Int("failed_queries", failures),
		zap.Int("results", len(results)),
		zap.Strings("providers", sortedOpenAIResponsesWebSearchLogValues(providers)),
		zap.Strings("unsupported_commands", plan.Unsupported),
	)
	if len(results) == 0 {
		return nil, nil
	}
	return &OpenAIForwardResult{
		Model:            requestedModel,
		UpstreamModel:    upstreamModel,
		UpstreamEndpoint: openAIAlphaSearchEmulationEndpoint,
		Duration:         time.Since(start),
		WebSearchCalls:   1,
	}, nil
}

// buildOpenAIAlphaSearchEmulationOutput 把各查询结果拼成模型可读的纯文本，并生成
// 与 PAT 路径一致的 text_result 列表；ref_id 跨查询连续编号，便于模型引用。
//
// @param blocks 各查询的结果块。
// @param unsupported 请求中出现但本地不执行的命令名。
// @param maxChars 输出字符上限，0 表示不限制。
// @return 纯文本 output 与 results 列表。
func buildOpenAIAlphaSearchEmulationOutput(blocks []openAIAlphaSearchEmulationBlock, unsupported []string, maxChars int) (string, []any) {
	var b strings.Builder
	results := make([]any, 0)
	for _, block := range blocks {
		if len(block.Results) == 0 {
			fmt.Fprintf(&b, "No search results found for %q.\n\n", block.Query)
			continue
		}
		fmt.Fprintf(&b, "Search results for %q:\n", block.Query)
		for _, result := range block.Results {
			refID := fmt.Sprintf("turn0search%d", len(results))
			fmt.Fprintf(&b, "%d. [%s] %s\n%s\n", len(results)+1, refID, result.Title, result.URL)
			if snippet := strings.TrimSpace(result.Snippet); snippet != "" {
				_, _ = b.WriteString(snippet)
				_ = b.WriteByte('\n')
			}
			if pageAge := strings.TrimSpace(result.PageAge); pageAge != "" {
				fmt.Fprintf(&b, "Published: %s\n", pageAge)
			}
			_ = b.WriteByte('\n')
			entry := map[string]any{"type": "text_result", "ref_id": refID, "url": result.URL}
			if title := strings.TrimSpace(result.Title); title != "" {
				entry["title"] = title
			}
			results = append(results, entry)
		}
	}
	if len(unsupported) > 0 {
		fmt.Fprintf(&b, "Unsupported commands in this gateway: %s. Use the search results above, or fetch the page yourself.\n", strings.Join(unsupported, ", "))
	}
	output := strings.TrimSpace(b.String())
	if maxChars > 0 && utf8.RuneCountInString(output) > maxChars {
		runes := []rune(output)
		output = string(runes[:maxChars]) + "\n...<truncated>"
	}
	return output, results
}

// writeOpenAIAlphaSearchEmulationResponse 把本地模拟结果按 alpha/search 响应形态写回下游。
//
// @param c Gin 请求上下文。
// @param output 模型可读文本。
// @param results text_result 列表，可为空。
// @return 编码失败时返回错误。
func writeOpenAIAlphaSearchEmulationResponse(c *gin.Context, output string, results []any) error {
	body, err := encodeOpenAIAlphaSearchResponse(output, results)
	if err != nil {
		return err
	}
	c.Data(http.StatusOK, "application/json", body)
	return nil
}

// writeOpenAIAlphaSearchFailed 写回 502 的 web_search_failed 错误，形态与 Responses
// 模拟搜索路径一致。
//
// @param c Gin 请求上下文。
// @param message 错误说明。
func writeOpenAIAlphaSearchFailed(c *gin.Context, message string) {
	writeOpenAIResponsesWebSearchError(c, http.StatusBadGateway, "web_search_failed", message, "tools")
}
