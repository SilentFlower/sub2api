package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	debugGatewayBodyEnv             = "SUB2API_DEBUG_GATEWAY_BODY"
	debugGatewayBodyDefaultFilename = "gateway_debug.log"
)

// initDebugGatewayBodyFile 初始化网关调试日志文件。
//
//   - "1"/"true" 等布尔值：当前目录下 gateway_debug.log
//   - 已有目录路径：该目录下 gateway_debug.log
//   - 其他值：视为完整文件路径
func initDebugGatewayBodyFile(file *atomic.Pointer[os.File], path string) {
	path = resolveGatewayDebugSnapshotPath(path)

	// 显式文件路径可能包含尚不存在的父目录，初始化时一并创建。
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil { //nolint:gosec // G703: 路径来自启动环境变量 SUB2API_DEBUG_GATEWAY_BODY，属于运维配置。
			slog.Error("failed to create gateway debug log directory", "dir", dir, "error", err)
			return
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644) //nolint:gosec // G703: 路径来自运维配置，不接受请求输入。
	if err != nil {
		slog.Error("failed to open gateway debug log file", "path", path, "error", err)
		return
	}
	file.Store(f)
	slog.Info("gateway debug logging enabled", "path", path)
}

func resolveGatewayDebugSnapshotPath(path string) string {
	if parseDebugEnvBool(path) {
		return debugGatewayBodyDefaultFilename
	}
	// 目录路径自动追加默认文件名，保持环境变量既可填目录也可填文件。
	if info, err := os.Stat(path); err == nil && info.IsDir() { //nolint:gosec // G703: 路径来自运维配置，不接受请求输入。
		return filepath.Join(path, debugGatewayBodyDefaultFilename)
	}
	return path
}

func (s *GatewayService) initDebugGatewayBodyFile(path string) {
	initDebugGatewayBodyFile(&s.debugGatewayBodyFile, path)
}

// debugLogGatewaySnapshot 将网关请求的完整快照写入独立调试日志文件。
// 快照包含请求上下文、脱敏后的请求头和完整请求体。
func (s *GatewayService) debugLogGatewaySnapshot(tag string, headers http.Header, body []byte, extra map[string]string) {
	debugLogGatewaySnapshot(&s.debugGatewayBodyFile, tag, headers, body, extra)
}

func debugLogGatewaySnapshot(file *atomic.Pointer[os.File], tag string, headers http.Header, body []byte, extra map[string]string) {
	f := file.Load()
	if f == nil {
		return
	}

	var buf strings.Builder
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	fmt.Fprintf(&buf, "\n========== [%s] %s ==========\n", ts, tag)

	// 上下文键排序后输出，保证相同请求快照便于直接比较。
	if len(extra) > 0 {
		fmt.Fprint(&buf, "--- context ---\n")
		extraKeys := make([]string, 0, len(extra))
		for key := range extra {
			extraKeys = append(extraKeys, key)
		}
		sort.Strings(extraKeys)
		for _, key := range extraKeys {
			fmt.Fprintf(&buf, "  %s: %s\n", key, extra[key])
		}
	}

	// 请求头按真实客户端 wire 顺序输出，并对认证信息脱敏。
	fmt.Fprint(&buf, "--- headers ---\n")
	for _, key := range sortHeadersByWireOrder(headers) {
		for _, value := range headers[key] {
			fmt.Fprintf(&buf, "  %s: %s\n", key, safeHeaderValueForLog(key, value))
		}
	}

	fmt.Fprint(&buf, "--- body ---\n")
	if len(body) == 0 {
		fmt.Fprint(&buf, "  (empty)\n")
	} else {
		var pretty bytes.Buffer
		if json.Indent(&pretty, body, "  ", "  ") == nil {
			fmt.Fprintf(&buf, "  %s\n", pretty.Bytes())
		} else {
			// 非 JSON 请求体必须原样保留，避免调试信息丢失。
			fmt.Fprintf(&buf, "  %s\n", body)
		}
	}

	// 调试文件允许并发写入交错；单次快照仍以一个字符串完成写入。
	_, _ = f.WriteString(buf.String())
}
