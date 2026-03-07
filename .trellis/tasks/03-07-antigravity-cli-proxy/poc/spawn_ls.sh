#!/usr/bin/env bash
# Headless Language Server PoC 启动脚本
# 用法: ./spawn_ls.sh [oauth_token]
#
# 启动一个 headless language_server 实例，模拟真实 Antigravity IDE 环境。
# 需要先运行 stub_server.go 作为 extension server。

set -euo pipefail

LS_BIN="/usr/share/antigravity/resources/app/extensions/antigravity/bin/language_server_linux_x64"
APP_ROOT="/usr/share/antigravity"

# 端口配置（使用固定端口方便调试，生产环境应随机分配）
SERVER_PORT="${SERVER_PORT:-42200}"
LSP_PORT="${LSP_PORT:-42201}"
EXT_SERVER_PORT="${EXT_SERVER_PORT:-42202}"
CSRF_TOKEN="${CSRF_TOKEN:-$(uuidgen)}"

# 数据目录
DATA_DIR="${DATA_DIR:-/tmp/ls-poc}"
GEMINI_DIR="${DATA_DIR}/.gemini"
APP_DATA_DIR="${GEMINI_DIR}/antigravity"

# Parent pipe（活性检测）
PIPE_HEX=$(head -c 8 /dev/urandom | xxd -p)
PARENT_PIPE_PATH="${DATA_DIR}/server_${PIPE_HEX}"

echo "=== Headless LS PoC ==="
echo "  LS binary:    ${LS_BIN}"
echo "  Server port:  ${SERVER_PORT}"
echo "  LSP port:     ${LSP_PORT}"
echo "  Ext server:   ${EXT_SERVER_PORT}"
echo "  CSRF token:   ${CSRF_TOKEN:0:8}..."
echo "  Data dir:     ${DATA_DIR}"
echo "  Pipe path:    ${PARENT_PIPE_PATH}"
echo ""

# 1. 准备数据目录
mkdir -p "${APP_DATA_DIR}/annotations" "${APP_DATA_DIR}/brain"
chmod -R 777 "${DATA_DIR}"

# 2. 预置 user_settings.pb（detectAndUseProxy = ENABLED）
printf '\x0a\x06\x0a\x00\x12\x02\x08\x01' > "${APP_DATA_DIR}/user_settings.pb"
echo "[OK] user_settings.pb 已写入"

# 3. 创建 parent pipe socket
# LS 会连接这个 socket 并阻塞读取。断开 = LS 退出。
# 这里用 socat 保持 socket 打开。
if command -v socat &>/dev/null; then
    socat UNIX-LISTEN:"${PARENT_PIPE_PATH}",fork /dev/null &
    SOCAT_PID=$!
    echo "[OK] Parent pipe socket 已创建 (socat PID: ${SOCAT_PID})"
else
    echo "[WARN] socat 未安装，跳过 parent pipe（LS 可能会意外退出）"
    PARENT_PIPE_PATH=""
fi

# 4. 构建启动参数
LS_ARGS=(
    "--enable_lsp"
    "--lsp_port=${LSP_PORT}"
    "--extension_server_port=${EXT_SERVER_PORT}"
    "--csrf_token=${CSRF_TOKEN}"
    "--server_port=${SERVER_PORT}"
    "--workspace_id=file_home_user_workspace"
    "--cloud_code_endpoint=https://daily-cloudcode-pa.googleapis.com"
    "--app_data_dir=antigravity"
    "--gemini_dir=${GEMINI_DIR}"
)

# 如果有 parent pipe 路径
if [ -n "${PARENT_PIPE_PATH}" ]; then
    LS_ARGS+=("--parent_pipe_path=${PARENT_PIPE_PATH}")
fi

# 5. 设置环境变量（模拟 Electron 宿主）
export ELECTRON_RUN_AS_NODE=1
export VSCODE_PID=$$
export VSCODE_CWD="$(pwd)"
export VSCODE_CRASH_REPORTER_PROCESS_TYPE=extensionHost
export VSCODE_HANDLES_UNCAUGHT_ERRORS=true
export SBX_CHROME_API_RQ=1
export CHROME_DESKTOP=antigravity.desktop
export NO_PROXY="*.azureedge.net"
export FC_FONTATIONS=1
export ANTIGRAVITY_SENTRY_SAMPLE_RATE=0
export ANTIGRAVITY_EDITOR_APP_ROOT="${APP_ROOT}"

# Go 调试（可选，取消注释以查看 HTTP/2 流量）
# export GODEBUG=http2debug=2

echo ""
echo "=== 启动 Language Server ==="
echo "  命令: ${LS_BIN} ${LS_ARGS[*]}"
echo ""

# 6. 启动 LS（前台运行，方便观察日志）
exec "${LS_BIN}" "${LS_ARGS[@]}"
