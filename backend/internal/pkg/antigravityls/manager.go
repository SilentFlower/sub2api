package antigravityls

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const (
	// DefaultLSBinary 默认 LS 二进制路径（Docker 镜像内）
	DefaultLSBinary = "/app/resources/antigravityls/language_server_linux_x64"

	// DefaultCloudCodeEndpoint 默认 Cloud Code API 端点
	DefaultCloudCodeEndpoint = "https://cloudcode-pa.googleapis.com"

	// tokenFileName LS standalone 模式读取的 OAuth token 文件名
	tokenFileName = "jetski-standalone-oauth-token"

	// lsStartTimeout LS 启动超时时间
	lsStartTimeout = 15 * time.Second

	// heartbeatInterval 健康检查间隔
	heartbeatInterval = 30 * time.Second

	// defaultPortStart 默认起始端口
	defaultPortStart = 44000
)

// oauthToken Go oauth2.Token 兼容的 JSON 结构
type oauthToken struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
}

// Manager 管理 Language Server 实例的生命周期
type Manager struct {
	mu          sync.RWMutex
	instances   map[int64]*LSInstance // accountID -> instance
	client      *Client
	nextPort    int    // 下一个可用端口
	lsBinary    string // LS 二进制路径
	baseDataDir string // 基础数据目录
	proxyAddr   string // HTTP/SOCKS5 代理地址 (host:port)
}

// NewManager 创建 LS 管理器
// lsBinary: LS 二进制路径（空字符串则使用默认值）
// baseDataDir: 基础数据目录，每个账号在此下创建子目录
// proxyAddr: HTTP 代理地址（如 "127.0.0.1:7890"），用于 LS 的 REST 请求
func NewManager(lsBinary, baseDataDir, proxyAddr string) *Manager {
	if lsBinary == "" {
		lsBinary = DefaultLSBinary
	}
	return &Manager{
		instances:   make(map[int64]*LSInstance),
		client:      NewClient(),
		nextPort:    defaultPortStart,
		lsBinary:    lsBinary,
		baseDataDir: baseDataDir,
		proxyAddr:   proxyAddr,
	}
}

// GetOrStartInstance 获取已存在的 LS 实例，如果不存在或已停止则启动新的
// refreshToken 和 accessToken 用于写入 OAuth token 文件供 LS 读取
func (m *Manager) GetOrStartInstance(ctx context.Context, accountID int64, refreshToken, accessToken string) (*LSInstance, error) {
	// 先尝试获取已有实例
	m.mu.RLock()
	inst, ok := m.instances[accountID]
	m.mu.RUnlock()

	if ok && inst.IsRunning() {
		// 更新 token 文件（access_token 可能已刷新）
		if err := m.writeTokenFile(inst.DataDir, refreshToken, accessToken); err != nil {
			slog.Warn("更新 LS token 文件失败", "accountID", accountID, "error", err)
		}
		return inst, nil
	}

	// 需要启动新实例
	m.mu.Lock()
	defer m.mu.Unlock()

	// 双重检查（可能在等锁期间另一个 goroutine 已启动）
	if inst, ok := m.instances[accountID]; ok && inst.IsRunning() {
		return inst, nil
	}

	// 清理旧实例（如果存在）
	if inst, ok := m.instances[accountID]; ok {
		m.cleanupInstanceLocked(inst)
		delete(m.instances, accountID)
	}

	// 分配端口和数据目录
	port := m.allocatePort()
	dataDir := filepath.Join(m.baseDataDir, fmt.Sprintf("account_%d", accountID))

	// 写入 OAuth token 文件
	if err := m.writeTokenFile(dataDir, refreshToken, accessToken); err != nil {
		return nil, fmt.Errorf("写入 token 文件失败: %w", err)
	}

	// 启动 LS 进程
	inst, err := m.startLS(ctx, accountID, port, dataDir)
	if err != nil {
		return nil, fmt.Errorf("启动 LS 失败: %w", err)
	}

	m.instances[accountID] = inst

	slog.Info("LS 实例启动成功",
		"accountID", accountID,
		"port", port,
		"pid", inst.Process.Pid,
		"dataDir", dataDir,
	)

	return inst, nil
}

// StopInstance 停止指定账号的 LS 实例
func (m *Manager) StopInstance(accountID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst, ok := m.instances[accountID]
	if !ok {
		return nil
	}

	m.cleanupInstanceLocked(inst)
	delete(m.instances, accountID)

	slog.Info("LS 实例已停止", "accountID", accountID)
	return nil
}

// StopAll 停止所有 LS 实例
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, inst := range m.instances {
		m.cleanupInstanceLocked(inst)
		slog.Info("LS 实例已停止", "accountID", id)
	}
	m.instances = make(map[int64]*LSInstance)
}

// startLS 启动 Language Server 进程
func (m *Manager) startLS(ctx context.Context, accountID int64, port int, dataDir string) (*LSInstance, error) {
	// 构建启动命令
	cmd := exec.Command(m.lsBinary,
		"--standalone",
		"--app_data_dir="+filepath.Base(dataDir),
		fmt.Sprintf("--server_port=%d", port),
		"--cloud_code_endpoint="+DefaultCloudCodeEndpoint,
	)

	// 设置工作目录为数据目录的父目录
	// LS 的 --app_data_dir 是相对于工作目录的
	cmd.Dir = filepath.Dir(dataDir)

	// 设置环境变量
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("HTTPS_PROXY=http://%s", m.proxyAddr),
		fmt.Sprintf("HTTP_PROXY=http://%s", m.proxyAddr),
		"NO_PROXY=127.0.0.1,localhost",
		// gemini_dir 指向数据目录下的 .gemini 子目录
		fmt.Sprintf("HOME=%s", dataDir),
	)

	// 将 LS 的日志输出到 slog
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 LS 进程失败: %w", err)
	}

	baseURL := fmt.Sprintf("https://127.0.0.1:%d", port)

	// 等待 LS 就绪
	readyCtx, readyCancel := context.WithTimeout(ctx, lsStartTimeout)
	defer readyCancel()

	if err := m.waitForReady(readyCtx, baseURL); err != nil {
		// LS 启动失败，清理进程
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("等待 LS 就绪超时: %w", err)
	}

	// 创建健康检查 context
	healthCtx, healthCancel := context.WithCancel(context.Background())

	inst := &LSInstance{
		Port:       port,
		DataDir:    dataDir,
		AccountID:  accountID,
		Process:    cmd.Process,
		StartedAt:  time.Now(),
		BaseURL:    baseURL,
		cancelFunc: healthCancel,
	}

	// 启动健康检查和进程监控
	go m.monitorProcess(healthCtx, inst, cmd)

	return inst, nil
}

// waitForReady 轮询 Heartbeat 直到 LS 就绪
func (m *Manager) waitForReady(ctx context.Context, baseURL string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("LS 就绪等待超时")
		case <-ticker.C:
			if err := m.client.Heartbeat(ctx, baseURL); err == nil {
				return nil
			}
		}
	}
}

// monitorProcess 监控 LS 进程状态，进程退出后自动清理
func (m *Manager) monitorProcess(ctx context.Context, inst *LSInstance, cmd *exec.Cmd) {
	// 等待进程退出
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// 被主动停止
		_ = cmd.Process.Kill()
		<-waitDone
	case err := <-waitDone:
		// 进程意外退出
		slog.Error("LS 进程意外退出",
			"accountID", inst.AccountID,
			"pid", inst.Process.Pid,
			"error", err,
		)
		// 从实例表中移除
		m.mu.Lock()
		if current, ok := m.instances[inst.AccountID]; ok && current == inst {
			delete(m.instances, inst.AccountID)
		}
		m.mu.Unlock()
	}
}

// writeTokenFile 写入 OAuth token 文件供 LS 读取
// 路径: {dataDir}/.gemini/jetski-standalone-oauth-token
func (m *Manager) writeTokenFile(dataDir, refreshToken, accessToken string) error {
	geminiDir := filepath.Join(dataDir, ".gemini")
	if err := os.MkdirAll(geminiDir, 0755); err != nil {
		return fmt.Errorf("创建 .gemini 目录失败: %w", err)
	}

	token := oauthToken{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		RefreshToken: refreshToken,
		Expiry:       time.Now().Add(1 * time.Hour),
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 token 失败: %w", err)
	}

	tokenPath := filepath.Join(geminiDir, tokenFileName)
	if err := os.WriteFile(tokenPath, data, 0600); err != nil {
		return fmt.Errorf("写入 token 文件失败: %w", err)
	}

	return nil
}

// allocatePort 分配下一个可用端口
func (m *Manager) allocatePort() int {
	port := m.nextPort
	m.nextPort++
	return port
}

// cleanupInstanceLocked 清理 LS 实例（调用方须持有锁）
func (m *Manager) cleanupInstanceLocked(inst *LSInstance) {
	if inst.cancelFunc != nil {
		inst.cancelFunc()
	}
	if inst.Process != nil {
		_ = inst.Process.Kill()
	}
}
