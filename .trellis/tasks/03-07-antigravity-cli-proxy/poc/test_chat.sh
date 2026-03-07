#!/usr/bin/env bash
# =============================================================================
# 完整对话测试脚本
#
# 用法：
#   ./test_chat.sh [server_port] [message]
#
# 示例：
#   ./test_chat.sh 44000 "What is 2+2?"
#   ./test_chat.sh 44000 "用中文回答：什么是递归？"
# =============================================================================
set -euo pipefail

PORT="${1:-44000}"
MESSAGE="${2:-What is 2+2? Answer briefly.}"
MODEL="${MODEL:-MODEL_PLACEHOLDER_M18}"  # Gemini 3 Flash（默认）
BASE_URL="https://127.0.0.1:${PORT}/exa.language_server_pb.LanguageServerService"

echo "=== 对话测试 ==="
echo "  LS 端口:  ${PORT}"
echo "  模型:     ${MODEL}"
echo "  消息:     ${MESSAGE}"
echo ""

# --- Step 1: 获取可用模型 ---
echo "[Step 1] 获取可用模型..."
MODELS=$(curl -sk "${BASE_URL}/GetCascadeModelConfigData" \
    -H "Content-Type: application/json" \
    -H "Connect-Protocol-Version: 1" \
    -d '{}')
echo "${MODELS}" | python3 -c "
import sys, json
data = json.load(sys.stdin)
configs = data.get('clientModelConfigs', [])
if not configs:
    print('  (无模型返回)')
for c in configs:
    ma = c.get('modelOrAlias', {})
    key = ma.get('model', 'N/A') if isinstance(ma, dict) else str(ma)
    name = c.get('displayName', key)
    print(f'    - {key}: {name}')
" 2>/dev/null || echo "  (解析失败)"
echo ""

# --- Step 2: StartCascade ---
echo "[Step 2] 创建 Cascade..."
START_RESP=$(curl -sk "${BASE_URL}/StartCascade" \
    -H "Content-Type: application/json" \
    -H "Connect-Protocol-Version: 1" \
    -d '{}')
CASCADE_ID=$(echo "${START_RESP}" | python3 -c "import sys,json; print(json.load(sys.stdin)['cascadeId'])" 2>/dev/null)

if [ -z "${CASCADE_ID}" ]; then
    echo "  [!] StartCascade 失败: ${START_RESP}"
    exit 1
fi
echo "  [+] cascadeId: ${CASCADE_ID}"
echo ""

# --- Step 3: SendUserCascadeMessage ---
echo "[Step 3] 发送消息..."
SEND_RESP=$(curl -sk "${BASE_URL}/SendUserCascadeMessage" \
    -H "Content-Type: application/json" \
    -H "Connect-Protocol-Version: 1" \
    -d "{
        \"cascadeId\": \"${CASCADE_ID}\",
        \"cascadeConfig\": {
            \"plannerConfig\": {
                \"google\": {},
                \"planModel\": \"${MODEL}\",
                \"requestedModel\": {\"model\": \"${MODEL}\"}
            }
        },
        \"items\": [{\"textOrScopeItem\": {\"text\": \"${MESSAGE}\"}}],
        \"clientType\": \"IDE\",
        \"messageOrigin\": \"IDE\"
    }")
echo "  [+] 响应: ${SEND_RESP}"
echo ""

# --- Step 4: 轮询 GetCascadeTrajectory ---
echo "[Step 4] 等待 AI 响应..."
MAX_WAIT=60
ELAPSED=0
while [ ${ELAPSED} -lt ${MAX_WAIT} ]; do
    TRAJ=$(curl -sk "${BASE_URL}/GetCascadeTrajectory" \
        -H "Content-Type: application/json" \
        -H "Connect-Protocol-Version: 1" \
        -d "{\"cascadeId\": \"${CASCADE_ID}\"}")

    # 检查是否有 PLANNER_RESPONSE 类型的 step（已完成）
    HAS_PLANNER=$(echo "${TRAJ}" | python3 -c "
import sys, json
data = json.load(sys.stdin)
# 轨迹可能在 trajectory.steps 或直接 steps
steps = data.get('trajectory', data).get('steps', data.get('steps', []))
for s in steps:
    t = s.get('type', '')
    st = s.get('status', s.get('state', ''))
    if 'PLANNER_RESPONSE' in t and 'DONE' in st:
        print('YES')
        break
" 2>/dev/null)

    if [ "${HAS_PLANNER}" = "YES" ]; then
        echo "  [+] AI 响应完成！"
        echo ""
        echo "=== AI 回复 ==="
        echo "${TRAJ}" | python3 -c "
import sys, json
data = json.load(sys.stdin)
steps = data.get('trajectory', data).get('steps', data.get('steps', []))
for s in steps:
    t = s.get('type', '')
    if 'PLANNER_RESPONSE' in t:
        # 内容可能在 plannerResponse 或 content 字段
        pr = s.get('plannerResponse', s.get('content', {}))
        thinking = pr.get('thinking', '')
        # 文本可能在多种 key 中
        text = pr.get('text', '')
        # 如果有 toolCalls，也显示
        tool_calls = pr.get('toolCalls', [])
        if thinking:
            print('[Thinking]')
            print(thinking[:500])
            if len(thinking) > 500: print('...')
            print()
        if text:
            print('[Response]')
            print(text)
        elif tool_calls:
            print('[Tool Calls]')
            for tc in tool_calls:
                print(f'  - {tc.get(\"name\", \"unknown\")}: {tc.get(\"argumentsJson\", \"\")[:200]}')
        break
"
        echo ""
        echo "=== 完成 ==="
        exit 0
    fi

    sleep 2
    ELAPSED=$((ELAPSED + 2))
    printf "  ... 等待中 (%ds/%ds)\r" ${ELAPSED} ${MAX_WAIT}
done

echo ""
echo "  [!] 超时（${MAX_WAIT}s），最后获取的轨迹："
echo "${TRAJ}" | python3 -m json.tool 2>/dev/null || echo "${TRAJ}"
exit 1
