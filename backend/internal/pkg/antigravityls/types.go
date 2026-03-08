// Package antigravityls 提供 headless Language Server 代理功能。
// 通过启动 standalone 模式的 language_server 二进制，以 ConnectRPC 协议与 LS 通信，
// 代替 sub2api 直接向 Google API 发起 HTTP 请求。
package antigravityls

import (
	"context"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
)

// --- ConnectRPC 请求/响应类型 ---

// StartCascadeResponse StartCascade RPC 返回的 cascadeId
type StartCascadeResponse struct {
	CascadeID string `json:"cascadeId"`
}

// SendUserCascadeMessageRequest 发送用户消息请求
type SendUserCascadeMessageRequest struct {
	CascadeID     string         `json:"cascadeId"`
	CascadeConfig *CascadeConfig `json:"cascadeConfig,omitempty"`
	Items         []CascadeItem  `json:"items"`
	ClientType    string         `json:"clientType"`    // "IDE"
	MessageOrigin string         `json:"messageOrigin"` // "IDE"
}

// CascadeConfig Cascade 配置
type CascadeConfig struct {
	PlannerConfig *PlannerConfig `json:"plannerConfig,omitempty"`
}

// PlannerConfig Planner 配置（指定模型）
type PlannerConfig struct {
	Google         map[string]any  `json:"google,omitempty"`
	PlanModel      string          `json:"planModel"`
	RequestedModel *RequestedModel `json:"requestedModel,omitempty"`
}

// RequestedModel 请求的模型
type RequestedModel struct {
	Model string `json:"model"`
}

// CascadeItem Cascade 消息项
type CascadeItem struct {
	TextOrScopeItem *TextOrScopeItem `json:"textOrScopeItem,omitempty"`
}

// TextOrScopeItem 文本消息项
type TextOrScopeItem struct {
	Text string `json:"text"`
}

// --- Trajectory 响应类型 ---

// GetCascadeTrajectoryRequest 获取对话轨迹请求
type GetCascadeTrajectoryRequest struct {
	CascadeID string `json:"cascadeId"`
}

// GetCascadeTrajectoryResponse 获取对话轨迹响应
type GetCascadeTrajectoryResponse struct {
	Trajectory *Trajectory      `json:"trajectory,omitempty"`
	Steps      []TrajectoryStep `json:"steps,omitempty"` // 兼容：有时 steps 直接在根层
}

// GetSteps 获取 steps（兼容两种 JSON 结构）
func (r *GetCascadeTrajectoryResponse) GetSteps() []TrajectoryStep {
	if r.Trajectory != nil && len(r.Trajectory.Steps) > 0 {
		return r.Trajectory.Steps
	}
	return r.Steps
}

// Trajectory 对话轨迹
type Trajectory struct {
	Steps []TrajectoryStep `json:"steps,omitempty"`
}

// TrajectoryStep 轨迹步骤
type TrajectoryStep struct {
	Type            string           `json:"type"`                      // USER_INPUT, CONVERSATION_HISTORY, EPHEMERAL_MESSAGE, PLANNER_RESPONSE, CHECKPOINT, TOOL_RESULT, KNOWLEDGE_ARTIFACTS
	Status          string           `json:"status"`                    // IDLE, IN_PROGRESS, DONE
	PlannerResponse *PlannerResponse `json:"plannerResponse,omitempty"` // PLANNER_RESPONSE 类型的响应内容
	Content         *PlannerResponse `json:"content,omitempty"`         // 兼容：有时用 content 而非 plannerResponse
}

// NormalizedType 返回去掉 CORTEX_STEP_TYPE_ 前缀后的步骤类型。
func (s *TrajectoryStep) NormalizedType() string {
	return strings.TrimPrefix(strings.TrimSpace(s.Type), "CORTEX_STEP_TYPE_")
}

// NormalizedStatus 返回去掉 CORTEX_STEP_STATUS_ 前缀后的步骤状态。
func (s *TrajectoryStep) NormalizedStatus() string {
	return strings.TrimPrefix(strings.TrimSpace(s.Status), "CORTEX_STEP_STATUS_")
}

// IsType 检查步骤类型是否匹配，兼容带 CORTEX_STEP_TYPE_ 前缀的枚举值。
func (s *TrajectoryStep) IsType(expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return false
	}
	return s.NormalizedType() == expected || strings.Contains(s.Type, expected)
}

// IsStatus 检查步骤状态是否匹配，兼容带 CORTEX_STEP_STATUS_ 前缀的枚举值。
func (s *TrajectoryStep) IsStatus(expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return false
	}
	return s.NormalizedStatus() == expected || strings.Contains(s.Status, expected)
}

// GetPlannerResponse 获取 planner 响应（兼容两种 key）
func (s *TrajectoryStep) GetPlannerResponse() *PlannerResponse {
	if s.PlannerResponse != nil {
		return s.PlannerResponse
	}
	return s.Content
}

// PlannerResponse Planner 响应内容
type PlannerResponse struct {
	Text              string     `json:"text,omitempty"`
	Response          string     `json:"response,omitempty"`
	RawResponse       string     `json:"rawResponse,omitempty"`
	Thinking          string     `json:"thinking,omitempty"`
	ThinkingSignature string     `json:"thinkingSignature,omitempty"`
	ThinkingDuration  string     `json:"thinkingDuration,omitempty"`
	ToolCalls         []ToolCall `json:"toolCalls,omitempty"`
}

// GetText 返回 planner 响应中的最终文本，兼容 text/response/rawResponse 三种字段。
func (p *PlannerResponse) GetText() string {
	if p == nil {
		return ""
	}
	if strings.TrimSpace(p.Text) != "" {
		return p.Text
	}
	if strings.TrimSpace(p.Response) != "" {
		return p.Response
	}
	return p.RawResponse
}

// ToolCall 工具调用
type ToolCall struct {
	Name          string `json:"name"`
	ArgumentsJSON string `json:"argumentsJson,omitempty"`
}

// --- 模型配置 ---

// GetCascadeModelConfigDataResponse 获取可用模型列表响应
type GetCascadeModelConfigDataResponse struct {
	ClientModelConfigs []ClientModelConfig `json:"clientModelConfigs,omitempty"`
}

// ClientModelConfig 客户端模型配置
type ClientModelConfig struct {
	ModelOrAlias any    `json:"modelOrAlias"` // 可能是 map[string]any 或 string
	DisplayName  string `json:"displayName,omitempty"`
}

// --- LS 实例信息 ---

// LSInstance 代表一个运行中的 Language Server 实例
type LSInstance struct {
	Port       int                // LS 监听端口
	DataDir    string             // 数据目录（隔离不同账号）
	AccountID  int64              // 关联的账号 ID
	Process    *os.Process        // LS 进程句柄
	StartedAt  time.Time          // 启动时间
	BaseURL    string             // ConnectRPC base URL: https://127.0.0.1:{port}
	cancelFunc context.CancelFunc // 用于停止健康检查 goroutine
	mu         sync.Mutex         // 保护实例状态
}

// IsRunning 检查 LS 进程是否仍在运行
func (inst *LSInstance) IsRunning() bool {
	if inst.Process == nil {
		return false
	}
	// 向进程发送信号 0 检查是否存活
	err := inst.Process.Signal(syscall.Signal(0))
	return err == nil
}

// --- TCP Relay 实例 ---

// TCPRelay 通过 SOCKS5 代理转发 TCP 连接
type TCPRelay struct {
	listenAddr string        // 监听地址（如 "127.0.0.2:443"）
	proxyAddr  string        // SOCKS5 代理地址（如 "127.0.0.1:7890"）
	targetHost string        // 目标主机（如 "cloudcode-pa.googleapis.com"）
	targetPort int           // 目标端口（如 443）
	listener   net.Listener  // TCP 监听器
	done       chan struct{} // 关闭信号
}
