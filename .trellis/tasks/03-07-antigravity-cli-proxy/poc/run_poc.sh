#!/usr/bin/env bash
# =============================================================================
# Headless LS Standalone PoC — 一键启动脚本
#
# 启动步骤：
#   1. 配置 DNS 劫持（/etc/hosts）
#   2. 启动 TCP Relay（gRPC 代理）
#   3. 启动 Language Server（standalone 模式）
#
# 前置条件：
#   - Antigravity 已安装（/usr/share/antigravity）
#   - SOCKS5 代理在 127.0.0.1:7890 监听
#   - OAuth token 已存放在 ~/.gemini/jetski-standalone-oauth-token
#   - 需要 root 权限（修改 /etc/hosts、绑定 443 端口）
#
# 用法：
#   sudo ./run_poc.sh
# =============================================================================
set -euo pipefail

LS_BIN="/usr/share/antigravity/resources/app/extensions/antigravity/bin/language_server_linux_x64"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

SOCKS5_HOST="${SOCKS5_HOST:-127.0.0.1}"
SOCKS5_PORT="${SOCKS5_PORT:-7890}"
LS_PORT="${LS_PORT:-44000}"
RELAY_IP="${RELAY_IP:-127.0.0.2}"

# Google API 域名
GOOGLE_DOMAINS=(
    "cloudcode-pa.googleapis.com"
    "daily-cloudcode-pa.googleapis.com"
    "play.googleapis.com"
)

echo "=== Headless LS Standalone PoC ==="
echo "  LS binary:     ${LS_BIN}"
echo "  LS port:       ${LS_PORT}"
echo "  SOCKS5 proxy:  ${SOCKS5_HOST}:${SOCKS5_PORT}"
echo "  Relay IP:      ${RELAY_IP}"
echo ""

# --- Step 1: DNS 劫持 ---
echo "[Step 1] 配置 DNS 劫持..."
for domain in "${GOOGLE_DOMAINS[@]}"; do
    if ! grep -q "${RELAY_IP} ${domain}" /etc/hosts 2>/dev/null; then
        echo "${RELAY_IP} ${domain}" >> /etc/hosts
        echo "  [+] 添加 ${RELAY_IP} ${domain}"
    else
        echo "  [=] ${domain} 已存在"
    fi
done

# --- Step 2: 启动 TCP Relay ---
echo ""
echo "[Step 2] 启动 TCP Relay..."

# 检查是否已有 relay 在运行
if ss -tlnp | grep -q "${RELAY_IP}:443"; then
    echo "  [=] TCP Relay 已在 ${RELAY_IP}:443 运行"
else
    python3 "${SCRIPT_DIR}/tcp_relay.py" &
    RELAY_PID=$!
    sleep 0.5
    if kill -0 ${RELAY_PID} 2>/dev/null; then
        echo "  [+] TCP Relay 启动成功 (PID: ${RELAY_PID})"
    else
        echo "  [!] TCP Relay 启动失败"
        exit 1
    fi
fi

# --- Step 3: 检查 OAuth Token ---
echo ""
echo "[Step 3] 检查 OAuth Token..."
TOKEN_PATH="${HOME}/.gemini/jetski-standalone-oauth-token"
if [ -f "${TOKEN_PATH}" ]; then
    echo "  [+] Token 文件存在: ${TOKEN_PATH}"
else
    echo "  [!] Token 文件不存在: ${TOKEN_PATH}"
    echo "  [!] 请先通过 OAuth 流程获取 token，格式为 Go oauth2.Token JSON"
    echo '  [!] 示例: {"access_token":"ya29.xxx","token_type":"Bearer","refresh_token":"1//xxx","expiry":"2026-..."}'
    exit 1
fi

# --- Step 4: 启动 Language Server ---
echo ""
echo "[Step 4] 启动 Language Server (standalone 模式)..."
echo "  命令: ${LS_BIN} --standalone --app_data_dir=antigravity --server_port=${LS_PORT}"
echo ""

HTTPS_PROXY="http://${SOCKS5_HOST}:${SOCKS5_PORT}" \
HTTP_PROXY="http://${SOCKS5_HOST}:${SOCKS5_PORT}" \
NO_PROXY="127.0.0.1,localhost" \
exec "${LS_BIN}" \
    --standalone \
    --app_data_dir=antigravity \
    --server_port="${LS_PORT}" \
    --cloud_code_endpoint=https://cloudcode-pa.googleapis.com
