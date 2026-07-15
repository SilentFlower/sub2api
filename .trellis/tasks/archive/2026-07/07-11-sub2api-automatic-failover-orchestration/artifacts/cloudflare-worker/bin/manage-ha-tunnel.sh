#!/usr/bin/env bash

set -Eeuo pipefail

usage() {
  cat <<'EOF'
用法：
  manage-ha-tunnel.sh create --node A|B --account-id <ID> --api-token-file <文件> --token-output <文件> [--dry-run]
  manage-ha-tunnel.sh status --node A|B --account-id <ID> --api-token-file <文件> --tunnel-id <UUID>

说明：
  create 创建独立的 remotely-managed Tunnel，并写入 api.havefun.eu.cc 对应的 ingress。
  status 只读取 Tunnel、ingress 和连接器状态，不修改 Cloudflare。
  A 固定转发到 127.0.0.1:8080，B 固定转发到 127.0.0.1:18080。
  本脚本不创建 api.havefun.eu.cc DNS，不修改任何已有 Tunnel。
EOF
}

die() {
  printf '错误：%s\n' "$*" >&2
  exit 1
}

require_value() {
  local name="$1"
  local value="$2"
  [ -n "${value}" ] || die "缺少 ${name}"
}

api_request() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local -a arguments
  arguments=(
    -sS
    -X "${method}"
    -H "Authorization: Bearer ${api_token}"
    -H 'Content-Type: application/json'
    "https://api.cloudflare.com/client/v4${path}"
  )
  if [ -n "${body}" ]; then
    arguments+=(--data "${body}")
  fi
  curl "${arguments[@]}"
}

require_success() {
  local response="$1"
  local operation="$2"
  if ! jq -e '.success == true' >/dev/null 2>&1 <<<"${response}"; then
    printf '%s\n' "${response}" | jq -c '{errors, messages}' >&2 2>/dev/null || true
    die "Cloudflare API 操作失败：${operation}"
  fi
}

action="${1:-}"
[ -n "${action}" ] || { usage; exit 1; }
case "${action}" in
  -h|--help|help) usage; exit 0 ;;
esac
shift

node=""
account_id=""
api_token_file=""
token_output=""
tunnel_id=""
dry_run=false
api_hostname="api.havefun.eu.cc"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --node) node="${2:-}"; shift 2 ;;
    --account-id) account_id="${2:-}"; shift 2 ;;
    --api-token-file) api_token_file="${2:-}"; shift 2 ;;
    --token-output) token_output="${2:-}"; shift 2 ;;
    --tunnel-id) tunnel_id="${2:-}"; shift 2 ;;
    --dry-run) dry_run=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知参数：$1" ;;
  esac
done

case "${node}" in
  A)
    tunnel_name="sub2api-ha-a"
    origin_service="http://127.0.0.1:8080"
    ;;
  B)
    tunnel_name="sub2api-ha-b"
    origin_service="http://127.0.0.1:18080"
    ;;
  *) die "--node 必须是 A 或 B" ;;
esac

require_value "--account-id" "${account_id}"
require_value "--api-token-file" "${api_token_file}"
[ -f "${api_token_file}" ] || die "API Token 文件不存在：${api_token_file}"
token_mode="$(stat -c '%a' "${api_token_file}")"
[ "$((8#${token_mode} & 8#077))" -eq 0 ] || die "API Token 文件不能允许组或其它用户访问"
api_token="$(tr -d '\r\n' < "${api_token_file}")"
require_value "API Token 文件内容" "${api_token}"
command -v curl >/dev/null 2>&1 || die "缺少 curl"
command -v jq >/dev/null 2>&1 || die "缺少 jq"

case "${action}" in
  create)
    require_value "--token-output" "${token_output}"
    list_response="$(api_request GET "/accounts/${account_id}/cfd_tunnel?is_deleted=false&name=${tunnel_name}")"
    require_success "${list_response}" "查询同名 Tunnel"
    existing_count="$(jq '[.result[] | select(.name == $name)] | length' --arg name "${tunnel_name}" <<<"${list_response}")"
    [ "${existing_count}" -eq 0 ] || die "已存在同名 Tunnel ${tunnel_name}，请先使用 status 核验，脚本不会复用或覆盖"

    if [ "${dry_run}" = "true" ]; then
      printf 'dry-run：将创建 %s，ingress=%s -> %s，Token 将写入 %s。\n' \
        "${tunnel_name}" "${api_hostname}" "${origin_service}" "${token_output}"
      exit 0
    fi

    create_body="$(jq -cn --arg name "${tunnel_name}" '{name:$name, config_src:"cloudflare"}')"
    create_response="$(api_request POST "/accounts/${account_id}/cfd_tunnel" "${create_body}")"
    require_success "${create_response}" "创建 Tunnel"
    tunnel_id="$(jq -er '.result.id' <<<"${create_response}")"

    config_body="$(jq -cn \
      --arg hostname "${api_hostname}" \
      --arg service "${origin_service}" \
      '{config:{ingress:[{hostname:$hostname,service:$service},{service:"http_status:404"}]}}')"
    config_response="$(api_request PUT "/accounts/${account_id}/cfd_tunnel/${tunnel_id}/configurations" "${config_body}")"
    if ! jq -e '.success == true' >/dev/null 2>&1 <<<"${config_response}"; then
      printf 'Tunnel 已创建但 ingress 配置失败，请保留现场并人工检查：%s\n' "${tunnel_id}" >&2
      require_success "${config_response}" "配置 Tunnel ingress"
    fi

    token_response="$(api_request GET "/accounts/${account_id}/cfd_tunnel/${tunnel_id}/token")"
    require_success "${token_response}" "读取 Tunnel Token"
    install -d -m 0700 "$(dirname -- "${token_output}")"
    token_temporary="${token_output}.tmp.$$"
    trap 'rm -f -- "${token_temporary}"' EXIT
    jq -er '.result' <<<"${token_response}" > "${token_temporary}"
    chmod 0600 "${token_temporary}"
    mv -f -- "${token_temporary}" "${token_output}"
    trap - EXIT
    printf '已创建 Tunnel：name=%s id=%s target=%s.cfargotunnel.com\n' \
      "${tunnel_name}" "${tunnel_id}" "${tunnel_id}"
    printf 'Token 已写入受限文件，不会输出到终端：%s\n' "${token_output}"
    ;;
  status)
    require_value "--tunnel-id" "${tunnel_id}"
    info_response="$(api_request GET "/accounts/${account_id}/cfd_tunnel/${tunnel_id}")"
    require_success "${info_response}" "读取 Tunnel"
    actual_name="$(jq -er '.result.name' <<<"${info_response}")"
    [ "${actual_name}" = "${tunnel_name}" ] || die "Tunnel 名称不匹配：期望 ${tunnel_name}，实际 ${actual_name}"

    config_response="$(api_request GET "/accounts/${account_id}/cfd_tunnel/${tunnel_id}/configurations")"
    require_success "${config_response}" "读取 Tunnel ingress"
    actual_hostname="$(jq -er '.result.config.ingress[0].hostname' <<<"${config_response}")"
    actual_service="$(jq -er '.result.config.ingress[0].service' <<<"${config_response}")"
    catch_all="$(jq -er '.result.config.ingress[-1].service' <<<"${config_response}")"
    [ "${actual_hostname}" = "${api_hostname}" ] || die "ingress hostname 不匹配：期望 ${api_hostname}，实际 ${actual_hostname}"
    [ "${actual_service}" = "${origin_service}" ] || die "ingress 不匹配：期望 ${origin_service}，实际 ${actual_service}"
    [ "${catch_all}" = "http_status:404" ] || die "Tunnel 缺少安全 catch-all 404"

    connections_response="$(api_request GET "/accounts/${account_id}/cfd_tunnel/${tunnel_id}/connections")"
    require_success "${connections_response}" "读取 Tunnel 连接器"
    connector_count="$(jq '.result | length' <<<"${connections_response}")"
    tunnel_status="$(jq -r '.result.status // "unknown"' <<<"${info_response}")"
    printf 'Tunnel 只读核验通过：name=%s id=%s status=%s connectors=%s ingress=%s->%s target=%s.cfargotunnel.com\n' \
      "${tunnel_name}" "${tunnel_id}" "${tunnel_status}" "${connector_count}" \
      "${api_hostname}" "${origin_service}" "${tunnel_id}"
    ;;
  *) die "操作必须是 create 或 status" ;;
esac
