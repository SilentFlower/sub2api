//go:build unit

package service

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveGatewayDebugSnapshotPath(t *testing.T) {
	t.Run("布尔开关使用默认文件名", func(t *testing.T) {
		require.Equal(t, debugGatewayBodyDefaultFilename, resolveGatewayDebugSnapshotPath("true"))
	})

	t.Run("目录路径追加默认文件名", func(t *testing.T) {
		dir := t.TempDir()
		require.Equal(t, filepath.Join(dir, debugGatewayBodyDefaultFilename), resolveGatewayDebugSnapshotPath(dir))
	})

	t.Run("显式文件路径保持不变", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "custom.log")
		require.Equal(t, path, resolveGatewayDebugSnapshotPath(path))
	})
}

func TestDebugLogGatewaySnapshotDisabled(t *testing.T) {
	var file atomic.Pointer[os.File]
	require.NotPanics(t, func() {
		debugLogGatewaySnapshot(&file, "CLIENT_ORIGINAL", http.Header{}, []byte(`{"input":"hello"}`), nil)
	})
}

func TestDebugLogGatewaySnapshotWritesRedactedRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "gateway.log")
	var file atomic.Pointer[os.File]
	initDebugGatewayBodyFile(&file, path)
	opened := file.Load()
	require.NotNil(t, opened)

	headers := http.Header{
		"Authorization": {"Bearer secret-token"},
		"Content-Type":  {"application/json"},
	}
	debugLogGatewaySnapshot(&file, "UPSTREAM_FORWARD", headers, []byte(`{"input":"hello"}`), map[string]string{
		"model":   "gpt-5.5",
		"account": "42(test)",
	})
	require.NoError(t, opened.Close())

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	text := string(content)
	require.Contains(t, text, "UPSTREAM_FORWARD")
	require.Contains(t, text, "account: 42(test)")
	require.Contains(t, text, "model: gpt-5.5")
	require.Contains(t, text, "Bearer [redacted]")
	require.Contains(t, text, `"input": "hello"`)
	require.False(t, strings.Contains(text, "secret-token"))
}
