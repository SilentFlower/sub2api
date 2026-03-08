package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravityls"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AntigravityLSGatewayService 通过本地 Language Server 代理处理 Antigravity 请求
// 与 AntigravityGatewayService 的区别是：不直接 HTTP 调 Google API，而是通过本地 LS 进程中转
type AntigravityLSGatewayService struct {
	lsManager     *antigravityls.Manager
	lsClient      *antigravityls.Client
	tokenProvider *AntigravityTokenProvider
}

// NewAntigravityLSGatewayService 创建 LS 代理网关 Service
func NewAntigravityLSGatewayService(
	tokenProvider *AntigravityTokenProvider,
	lsManager *antigravityls.Manager,
) *AntigravityLSGatewayService {
	return &AntigravityLSGatewayService{
		lsManager:     lsManager,
		lsClient:      antigravityls.NewClient(),
		tokenProvider: tokenProvider,
	}
}

// lsModelMapping Claude 请求模型 → LS Cascade 模型 key 的映射
// LS 使用 MODEL_PLACEHOLDER_xxx 格式的模型标识符
var lsModelMapping = map[string]string{
	// Claude 模型
	"claude-opus-4-6":            "MODEL_PLACEHOLDER_M26",
	"claude-opus-4-6-thinking":   "MODEL_PLACEHOLDER_M26",
	"claude-opus-4-5-thinking":   "MODEL_PLACEHOLDER_M26",
	"claude-sonnet-4-6":          "MODEL_PLACEHOLDER_M35",
	"claude-sonnet-4-5":          "MODEL_PLACEHOLDER_M35",
	"claude-sonnet-4-5-thinking": "MODEL_PLACEHOLDER_M35",
	// Gemini 模型
	"gemini-3.1-pro-high": "MODEL_PLACEHOLDER_M37",
	"gemini-3.1-pro-low":  "MODEL_PLACEHOLDER_M36",
	"gemini-3.1-pro":      "MODEL_PLACEHOLDER_M37",
	"gemini-3-flash":      "MODEL_PLACEHOLDER_M18",
	// GPT
	"gpt-oss-120b": "MODEL_OPENAI_GPT_OSS_120B_MEDIUM",
}

// defaultLSModel 默认使用的 LS 模型
const defaultLSModel = "MODEL_PLACEHOLDER_M18" // Gemini 3 Flash

// lsPollInterval Trajectory 轮询间隔
const lsPollInterval = 400 * time.Millisecond

// lsPollTimeout 轮询超时时间
const lsPollTimeout = 5 * time.Minute

// Forward 处理 Claude API 请求，通过本地 LS 代理转发
func (s *AntigravityLSGatewayService) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte, isStickySession bool) (*ForwardResult, error) {
	startTime := time.Now()
	prefix := fmt.Sprintf("[antigravityls-Forward] account=%s", account.Name)

	// 解析 Claude 请求
	var claudeReq antigravity.ClaudeRequest
	if err := json.Unmarshal(body, &claudeReq); err != nil {
		return nil, s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", "Invalid request body")
	}
	if strings.TrimSpace(claudeReq.Model) == "" {
		return nil, s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", "Missing model")
	}

	originalModel := claudeReq.Model

	// 映射到 LS 模型 key
	lsModel := s.mapToLSModel(claudeReq.Model)

	// 获取 access_token 和 refresh_token
	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		slog.Error("获取 access_token 失败", "prefix", prefix, "error", err)
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "authentication_error", "Failed to get upstream access token")
	}
	refreshToken := account.GetCredential("refresh_token")

	// 获取或启动 LS 实例
	inst, err := s.lsManager.GetOrStartInstance(ctx, account.ID, refreshToken, accessToken)
	if err != nil {
		slog.Error("获取 LS 实例失败", "prefix", prefix, "error", err)
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "api_error", "Failed to start Language Server")
	}

	slog.Info("LS 实例已就绪",
		"prefix", prefix,
		"port", inst.Port,
		"lsModel", lsModel,
	)

	// 构建用户消息文本（将 Claude Messages 拼接为纯文本）
	userText := s.buildUserText(&claudeReq)
	if userText == "" {
		return nil, s.writeClaudeError(c, http.StatusBadRequest, "invalid_request_error", "Empty message content")
	}

	// 创建 Cascade
	cascadeID, err := s.lsClient.StartCascade(ctx, inst.BaseURL)
	if err != nil {
		slog.Error("StartCascade 失败", "prefix", prefix, "error", err)
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "api_error", "Failed to create cascade")
	}

	// 发送用户消息
	sendReq := &antigravityls.SendUserCascadeMessageRequest{
		CascadeID: cascadeID,
		CascadeConfig: &antigravityls.CascadeConfig{
			PlannerConfig: &antigravityls.PlannerConfig{
				Google:         map[string]any{},
				PlanModel:      lsModel,
				RequestedModel: &antigravityls.RequestedModel{Model: lsModel},
			},
		},
		Items: []antigravityls.CascadeItem{
			{TextOrScopeItem: &antigravityls.TextOrScopeItem{Text: userText}},
		},
		ClientType:    "IDE",
		MessageOrigin: "IDE",
	}

	slog.Info("发送用户消息到 LS",
		"prefix", prefix,
		"cascadeId", cascadeID,
		"textLen", len(userText),
		"lsModel", lsModel,
	)

	if err := s.lsClient.SendUserCascadeMessage(ctx, inst.BaseURL, sendReq); err != nil {
		slog.Error("SendUserCascadeMessage 失败", "prefix", prefix, "cascadeId", cascadeID, "error", err)
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "api_error", "Failed to send message to Language Server")
	}

	slog.Info("用户消息发送成功，开始轮询 Trajectory",
		"prefix", prefix,
		"cascadeId", cascadeID,
		"stream", claudeReq.Stream,
	)

	// 根据客户端请求模式选择流式或非流式处理
	requestID := "req_" + uuid.New().String()
	pollCtx, pollCancel := context.WithTimeout(ctx, lsPollTimeout)
	defer pollCancel()

	if claudeReq.Stream {
		// 流式：轮询 Trajectory 并实时推送 SSE 事件
		usage, firstTokenMs, clientDisconnect, err := s.handleStreamingResponse(pollCtx, c, inst.BaseURL, cascadeID, originalModel, startTime)
		if err != nil {
			return nil, err
		}
		return &ForwardResult{
			RequestID:        requestID,
			Usage:            *usage,
			Model:            originalModel,
			Stream:           true,
			Duration:         time.Since(startTime),
			FirstTokenMs:     firstTokenMs,
			ClientDisconnect: clientDisconnect,
		}, nil
	}

	// 非流式：轮询到完成后一次性返回 JSON
	usage, err := s.handleNonStreamingResponse(pollCtx, c, inst.BaseURL, cascadeID, originalModel)
	if err != nil {
		return nil, err
	}
	return &ForwardResult{
		RequestID: requestID,
		Usage:     *usage,
		Model:     originalModel,
		Stream:    false,
		Duration:  time.Since(startTime),
	}, nil
}

// TestConnection 测试 LS 模式的 Antigravity 账号连通性。
// 该方法复用真实的 LS 请求链路，避免后台“测试账号”接口误走直连网关。
func (s *AntigravityLSGatewayService) TestConnection(ctx context.Context, account *Account, modelID string) (*TestConnectionResult, error) {
	if s == nil || s.tokenProvider == nil || s.lsManager == nil || s.lsClient == nil {
		return nil, fmt.Errorf("antigravity LS gateway service not configured")
	}

	testModelID := strings.TrimSpace(modelID)
	if testModelID == "" {
		testModelID = "claude-sonnet-4-5"
	}
	lsModel := s.mapToLSModel(testModelID)

	accessToken, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("获取 access_token 失败: %w", err)
	}
	refreshToken := account.GetCredential("refresh_token")

	inst, err := s.lsManager.GetOrStartInstance(ctx, account.ID, refreshToken, accessToken)
	if err != nil {
		return nil, fmt.Errorf("获取 LS 实例失败: %w", err)
	}

	cascadeID, err := s.lsClient.StartCascade(ctx, inst.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("StartCascade 失败: %w", err)
	}

	sendReq := &antigravityls.SendUserCascadeMessageRequest{
		CascadeID: cascadeID,
		CascadeConfig: &antigravityls.CascadeConfig{
			PlannerConfig: &antigravityls.PlannerConfig{
				Google:         map[string]any{},
				PlanModel:      lsModel,
				RequestedModel: &antigravityls.RequestedModel{Model: lsModel},
			},
		},
		Items: []antigravityls.CascadeItem{
			{TextOrScopeItem: &antigravityls.TextOrScopeItem{Text: "."}},
		},
		ClientType:    "IDE",
		MessageOrigin: "IDE",
	}

	if err := s.lsClient.SendUserCascadeMessage(ctx, inst.BaseURL, sendReq); err != nil {
		return nil, fmt.Errorf("SendUserCascadeMessage 失败: %w", err)
	}

	pollCtx, pollCancel := context.WithTimeout(ctx, lsPollTimeout)
	defer pollCancel()

	steps, err := s.lsClient.PollTrajectoryUntilDone(pollCtx, inst.BaseURL, cascadeID, lsPollInterval, nil)
	if err != nil {
		return nil, fmt.Errorf("轮询 Trajectory 失败: %w", err)
	}

	return &TestConnectionResult{
		Text:        extractLSTestResponseText(steps),
		MappedModel: lsModel,
	}, nil
}

// handleStreamingResponse 处理流式请求：轮询 Trajectory 并以 SSE 事件推送给客户端
func (s *AntigravityLSGatewayService) handleStreamingResponse(
	ctx context.Context,
	c *gin.Context,
	baseURL, cascadeID, originalModel string,
	startTime time.Time,
) (*ClaudeUsage, *int, bool, error) {
	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, nil, false, fmt.Errorf("streaming not supported")
	}

	transformer := antigravityls.NewTrajectoryTransformer(originalModel)
	var firstTokenMs *int
	clientDisconnect := false

	slog.Info("开始流式轮询 Trajectory",
		"cascadeId", cascadeID,
		"model", originalModel,
	)

	// 轮询 Trajectory 直到完成
	finalSteps, err := s.lsClient.PollTrajectoryUntilDone(ctx, baseURL, cascadeID, lsPollInterval,
		func(steps []antigravityls.TrajectoryStep) error {
			events := transformer.ProcessNewSteps(steps)
			if len(events) == 0 {
				return nil
			}

			slog.Debug("推送 SSE 事件",
				"cascadeId", cascadeID,
				"eventBytes", len(events),
			)

			// 记录首字时间
			if firstTokenMs == nil {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}

			// 写入 SSE 事件
			if _, err := c.Writer.Write(events); err != nil {
				clientDisconnect = true
				return fmt.Errorf("客户端已断开: %w", err)
			}
			flusher.Flush()
			return nil
		},
	)

	if err != nil && !clientDisconnect {
		// 轮询超时或其他错误
		slog.Error("轮询 Trajectory 失败", "cascadeId", cascadeID, "error", err)
	}

	usageData := antigravityls.ExtractClaudeUsage(finalSteps)

	// 发送结束事件
	finishEvents := transformer.Finish(transformer.StopReason(), usageData)
	if !clientDisconnect && len(finishEvents) > 0 {
		_, _ = c.Writer.Write(finishEvents)
		flusher.Flush()
	}

	usage := &ClaudeUsage{
		InputTokens:          usageData.InputTokens,
		OutputTokens:         usageData.OutputTokens,
		CacheReadInputTokens: usageData.CacheReadInputTokens,
	}
	return usage, firstTokenMs, clientDisconnect, nil
}

// handleNonStreamingResponse 处理非流式请求：轮询到完成后返回完整 JSON 响应
func (s *AntigravityLSGatewayService) handleNonStreamingResponse(
	ctx context.Context,
	c *gin.Context,
	baseURL, cascadeID, originalModel string,
) (*ClaudeUsage, error) {
	// 轮询 Trajectory 直到完成
	steps, err := s.lsClient.PollTrajectoryUntilDone(ctx, baseURL, cascadeID, lsPollInterval, nil)
	if err != nil {
		slog.Error("轮询 Trajectory 失败", "cascadeId", cascadeID, "error", err)
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "api_error", "Failed to get response from Language Server")
	}

	// 转换为 Claude 非流式响应
	claudeResp, err := antigravityls.TransformTrajectoryToClaude(steps, originalModel)
	if err != nil {
		return nil, s.writeClaudeError(c, http.StatusInternalServerError, "api_error", "Failed to transform response")
	}

	// 返回 JSON
	c.JSON(http.StatusOK, claudeResp)

	usage := &ClaudeUsage{
		InputTokens:          claudeResp.Usage.InputTokens,
		OutputTokens:         claudeResp.Usage.OutputTokens,
		CacheReadInputTokens: claudeResp.Usage.CacheReadInputTokens,
	}
	return usage, nil
}

// extractLSTestResponseText 提取测试请求的最终文本。
// LS 在轮询阶段会反复返回同一个 PLANNER_RESPONSE 的累积内容，因此这里取最后一个即可，避免重复拼接。
func extractLSTestResponseText(steps []antigravityls.TrajectoryStep) string {
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		if !step.IsType("PLANNER_RESPONSE") {
			continue
		}
		pr := step.GetPlannerResponse()
		if pr == nil {
			continue
		}
		if text := strings.TrimSpace(pr.GetText()); text != "" {
			return text
		}
		if strings.TrimSpace(pr.Thinking) != "" {
			return pr.Thinking
		}
	}
	return ""
}

// mapToLSModel 将 Claude/Gemini 模型名映射到 LS 的模型 key
func (s *AntigravityLSGatewayService) mapToLSModel(requestedModel string) string {
	// 先查精确匹配
	if lsModel, ok := lsModelMapping[requestedModel]; ok {
		return lsModel
	}
	// 再查前缀匹配
	for prefix, lsModel := range lsModelMapping {
		if strings.HasPrefix(requestedModel, prefix) {
			return lsModel
		}
	}
	return defaultLSModel
}

// buildUserText 从 Claude Messages 中提取用户消息文本
// 单轮纯文本请求直接返回文本，多轮/工具请求则展开为结构化对话转录。
func (s *AntigravityLSGatewayService) buildUserText(claudeReq *antigravity.ClaudeRequest) string {
	if claudeReq == nil {
		return ""
	}

	if text := extractSimpleLastUserText(claudeReq.Messages); text != "" && len(claudeReq.Messages) == 1 && len(claudeReq.Tools) == 0 && len(claudeReq.System) == 0 {
		return text
	}

	var sections []string
	if systemText := extractSystemText(claudeReq.System); systemText != "" {
		sections = append(sections, "[System]\n"+systemText)
	}
	if toolsText := renderToolsForLSTranscript(claudeReq.Tools); toolsText != "" {
		sections = append(sections, toolsText)
	}

	toolIDToName := make(map[string]string)
	for _, msg := range claudeReq.Messages {
		blockText := renderMessageForLSTranscript(msg, toolIDToName)
		if blockText == "" {
			continue
		}
		sections = append(sections, blockText)
	}

	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func extractSimpleLastUserText(messages []antigravity.ClaudeMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != "user" {
			continue
		}
		if text := extractPlainTextFromContent(msg.Content); text != "" {
			return text
		}
	}
	return ""
}

func extractSystemText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var blocks []antigravity.SystemBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func extractPlainTextFromContent(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var blocks []antigravity.ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func renderMessageForLSTranscript(msg antigravity.ClaudeMessage, toolIDToName map[string]string) string {
	var textContent string
	if err := json.Unmarshal(msg.Content, &textContent); err == nil {
		if strings.TrimSpace(textContent) == "" {
			return ""
		}
		return fmt.Sprintf("[%s]\n%s", normalizeTranscriptRole(msg.Role), strings.TrimSpace(textContent))
	}

	var blocks []antigravity.ContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return ""
	}

	var parts []string
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				parts = append(parts, block.Text)
			}
		case "thinking":
			if strings.TrimSpace(block.Thinking) != "" {
				parts = append(parts, "[Thinking]\n"+block.Thinking)
			}
		case "tool_use":
			toolName := translateToolNameForTranscript(block.Name)
			if block.ID != "" && block.Name != "" {
				toolIDToName[block.ID] = toolName
			}
			inputJSON := "{}"
			if block.Input != nil {
				if data, err := json.Marshal(block.Input); err == nil && len(data) > 0 {
					inputJSON = string(data)
				}
			}
			parts = append(parts, fmt.Sprintf("[Tool Call]\nname: %s\nid: %s\ninput: %s", toolName, block.ID, inputJSON))
		case "tool_result":
			toolName := translateToolNameForTranscript(block.Name)
			if toolName == "" {
				toolName = toolIDToName[block.ToolUseID]
			}
			resultText := parseToolResultTranscript(block.Content, block.IsError)
			parts = append(parts, fmt.Sprintf("[Tool Result]\nname: %s\nid: %s\ncontent: %s", toolName, block.ToolUseID, resultText))
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("[%s]\n%s", normalizeTranscriptRole(msg.Role), strings.Join(parts, "\n"))
}

func normalizeTranscriptRole(role string) string {
	switch role {
	case "assistant":
		return "Assistant"
	case "user":
		return "User"
	default:
		return role
	}
}

func translateToolNameForTranscript(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	return antigravityls.TranslateClientToolNameToAGForPrompt(trimmed)
}

func parseToolResultTranscript(content json.RawMessage, isError bool) string {
	if len(content) == 0 {
		if isError {
			return "Tool execution failed with no output."
		}
		return "Command executed successfully."
	}
	var text string
	if err := json.Unmarshal(content, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var blocks []antigravity.ContentBlock
	if err := json.Unmarshal(content, &blocks); err == nil {
		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
				parts = append(parts, block.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return strings.TrimSpace(string(content))
}

func renderToolsForLSTranscript(tools []antigravity.ClaudeTool) string {
	if len(tools) == 0 {
		return ""
	}
	lines := make([]string, 0, len(tools)+1)
	lines = append(lines, "[Available Tools]")
	for _, tool := range tools {
		clientName := strings.TrimSpace(tool.Name)
		if clientName == "" {
			continue
		}
		agName := antigravityls.TranslateClientToolNameToAGForPrompt(clientName)
		line := fmt.Sprintf("- client=%s, ag=%s", clientName, agName)
		description := strings.TrimSpace(tool.Description)
		if description == "" && tool.Custom != nil {
			description = strings.TrimSpace(tool.Custom.Description)
		}
		if description != "" {
			line += ": " + description
		}
		lines = append(lines, line)
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

// writeClaudeError 写入 Claude 格式的错误响应
func (s *AntigravityLSGatewayService) writeClaudeError(c *gin.Context, status int, errType, message string) error {
	c.JSON(status, antigravity.ClaudeError{
		Type: "error",
		Error: antigravity.ErrorDetail{
			Type:    errType,
			Message: message,
		},
	})
	return fmt.Errorf("%s: %s", errType, message)
}
