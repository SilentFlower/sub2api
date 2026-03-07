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

	if err := s.lsClient.SendUserCascadeMessage(ctx, inst.BaseURL, sendReq); err != nil {
		slog.Error("SendUserCascadeMessage 失败", "prefix", prefix, "cascadeId", cascadeID, "error", err)
		return nil, s.writeClaudeError(c, http.StatusBadGateway, "api_error", "Failed to send message to Language Server")
	}

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

	// 轮询 Trajectory 直到完成
	_, err := s.lsClient.PollTrajectoryUntilDone(ctx, baseURL, cascadeID, lsPollInterval,
		func(steps []antigravityls.TrajectoryStep) error {
			events := transformer.ProcessNewSteps(steps)
			if len(events) == 0 {
				return nil
			}

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

	// 发送结束事件
	stopReason := "end_turn"
	finishEvents := transformer.Finish(stopReason)
	if !clientDisconnect && len(finishEvents) > 0 {
		_, _ = c.Writer.Write(finishEvents)
		flusher.Flush()
	}

	usage := &ClaudeUsage{}
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
		InputTokens:  claudeResp.Usage.InputTokens,
		OutputTokens: claudeResp.Usage.OutputTokens,
	}
	return usage, nil
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
// 将最后一条 user 消息的所有 text 内容拼接
func (s *AntigravityLSGatewayService) buildUserText(claudeReq *antigravity.ClaudeRequest) string {
	// 从后往前查找最后一条 user 消息
	for i := len(claudeReq.Messages) - 1; i >= 0; i-- {
		msg := claudeReq.Messages[i]
		if msg.Role != "user" {
			continue
		}

		// content 可能是字符串或数组
		var textStr string
		if err := json.Unmarshal(msg.Content, &textStr); err == nil {
			return textStr
		}

		// 尝试解析为内容块数组
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		}
		if err := json.Unmarshal(msg.Content, &blocks); err == nil {
			var parts []string
			for _, b := range blocks {
				if b.Type == "text" && b.Text != "" {
					parts = append(parts, b.Text)
				}
			}
			return strings.Join(parts, "\n")
		}
	}
	return ""
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
