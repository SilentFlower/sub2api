package antigravityls

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	// connectRPCServicePath LanguageServerService 的 ConnectRPC 路径前缀
	connectRPCServicePath = "/exa.language_server_pb.LanguageServerService/"
)

// Client 与本地 Language Server 通信的 ConnectRPC 客户端
type Client struct {
	httpClient *http.Client
}

// NewClient 创建 ConnectRPC 客户端
// LS 在 localhost 使用自签名 HTTPS 证书，因此跳过 TLS 验证
func NewClient() *Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &Client{
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   30 * time.Second,
		},
	}
}

// rpc 发送 ConnectRPC JSON 请求到指定的 LS 实例
func (c *Client) rpc(ctx context.Context, baseURL, method string, reqBody any, resp any) error {
	url := baseURL + connectRPCServicePath + method

	var body io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("序列化请求失败: %w", err)
		}
		body = bytes.NewReader(data)
	} else {
		// 空请求体发送 {}
		body = bytes.NewReader([]byte("{}"))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// ConnectRPC 必需的 headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer httpResp.Body.Close()

	respData, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("RPC %s 返回 %d: %s", method, httpResp.StatusCode, string(respData))
	}

	if resp != nil {
		if err := json.Unmarshal(respData, resp); err != nil {
			return fmt.Errorf("反序列化响应失败: %w", err)
		}
	}

	return nil
}

// Heartbeat 心跳检查，验证 LS 是否就绪
func (c *Client) Heartbeat(ctx context.Context, baseURL string) error {
	return c.rpc(ctx, baseURL, "Heartbeat", nil, nil)
}

// StartCascade 创建新的对话，返回 cascadeId
func (c *Client) StartCascade(ctx context.Context, baseURL string) (string, error) {
	var resp StartCascadeResponse
	if err := c.rpc(ctx, baseURL, "StartCascade", nil, &resp); err != nil {
		return "", fmt.Errorf("StartCascade 失败: %w", err)
	}
	if resp.CascadeID == "" {
		return "", fmt.Errorf("StartCascade 返回空 cascadeId")
	}
	slog.Debug("创建 Cascade 成功", "cascadeId", resp.CascadeID)
	return resp.CascadeID, nil
}

// SendUserCascadeMessage 发送用户消息到指定的 Cascade
func (c *Client) SendUserCascadeMessage(ctx context.Context, baseURL string, req *SendUserCascadeMessageRequest) error {
	if err := c.rpc(ctx, baseURL, "SendUserCascadeMessage", req, nil); err != nil {
		return fmt.Errorf("SendUserCascadeMessage 失败: %w", err)
	}
	slog.Debug("发送用户消息成功", "cascadeId", req.CascadeID)
	return nil
}

// GetCascadeTrajectory 获取对话轨迹
func (c *Client) GetCascadeTrajectory(ctx context.Context, baseURL string, cascadeID string) (*GetCascadeTrajectoryResponse, error) {
	req := &GetCascadeTrajectoryRequest{CascadeID: cascadeID}
	var resp GetCascadeTrajectoryResponse
	if err := c.rpc(ctx, baseURL, "GetCascadeTrajectory", req, &resp); err != nil {
		return nil, fmt.Errorf("GetCascadeTrajectory 失败: %w", err)
	}
	return &resp, nil
}

// GetCascadeModelConfigData 获取可用模型列表
func (c *Client) GetCascadeModelConfigData(ctx context.Context, baseURL string) (*GetCascadeModelConfigDataResponse, error) {
	var resp GetCascadeModelConfigDataResponse
	if err := c.rpc(ctx, baseURL, "GetCascadeModelConfigData", nil, &resp); err != nil {
		return nil, fmt.Errorf("GetCascadeModelConfigData 失败: %w", err)
	}
	return &resp, nil
}

// PollTrajectoryUntilDone 轮询 Trajectory 直到 PLANNER_RESPONSE 完成或超时
// pollInterval: 轮询间隔（建议 300-500ms）
// onNewSteps: 每次发现新步骤时回调（用于流式推送）
func (c *Client) PollTrajectoryUntilDone(
	ctx context.Context,
	baseURL string,
	cascadeID string,
	pollInterval time.Duration,
	onNewSteps func(steps []TrajectoryStep) error,
) ([]TrajectoryStep, error) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var lastStepCount int

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			resp, err := c.GetCascadeTrajectory(ctx, baseURL, cascadeID)
			if err != nil {
				slog.Warn("轮询 Trajectory 失败，继续重试", "error", err, "cascadeId", cascadeID)
				continue
			}

			steps := resp.GetSteps()
			if len(steps) > lastStepCount {
				// 发现新步骤，通知回调
				if onNewSteps != nil {
					if err := onNewSteps(steps); err != nil {
						return steps, fmt.Errorf("处理新步骤失败: %w", err)
					}
				}
				lastStepCount = len(steps)
			}

			// 检查是否有 PLANNER_RESPONSE 已完成
			for _, step := range steps {
				if step.Type == "PLANNER_RESPONSE" && step.Status == "DONE" {
					return steps, nil
				}
			}
		}
	}
}
