#!/usr/bin/env bash

set -Eeuo pipefail

usage() {
  cat <<'EOF'
用法：
  install-agent.sh --node A|B --source <automation目录> --config <真实配置文件> [--dry-run]

说明：
  安装 HA agent 到 /opt/sub2api-ha-agent，并安装独立 systemd unit。
  本脚本不会创建 Cloudflare Tunnel、写入 Token、修改应用 restart policy 或启用 automatic。
EOF
}

die() {
  printf '错误：%s\n' "$*" >&2
  exit 1
}

node=""
source_dir=""
config_file=""
dry_run=false

while [ "$#" -gt 0 ]; do
  case "$1" in
    --node) node="${2:-}"; shift 2 ;;
    --source) source_dir="${2:-}"; shift 2 ;;
    --config) config_file="${2:-}"; shift 2 ;;
    --dry-run) dry_run=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知参数：$1" ;;
  esac
done

case "${node}" in A|B) ;; *) die "--node 必须是 A 或 B" ;; esac
[ -d "${source_dir}/sub2api_ha" ] || die "source 缺少 sub2api_ha 包"
[ -f "${source_dir}/bin/verify-action.sh" ] || die "source 缺少 bin/verify-action.sh"
if [ "${node}" = "A" ]; then
  [ -f "${source_dir}/bin/sync-release-if-needed.sh" ] || die "source 缺少 bin/sync-release-if-needed.sh"
  [ -f "${source_dir}/systemd/sub2api-ha-release-sync.service" ] || die "source 缺少发布同步 service"
  [ -f "${source_dir}/systemd/sub2api-ha-release-sync.timer" ] || die "source 缺少发布同步 timer"
fi
[ -f "${config_file}" ] || die "真实配置文件不存在：${config_file}"
[ "$(stat -c '%a' "${config_file}")" = "600" ] || die "真实配置文件权限必须为 600"
command -v python3 >/dev/null 2>&1 || die "缺少 python3"
config_node="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["node"])' "${config_file}")" \
  || die "无法读取配置中的 node"
[ "${config_node}" = "${node}" ] || die "配置 node=${config_node} 与 --node ${node} 不一致"

if [ "${dry_run}" = "true" ]; then
  printf 'dry-run：将安装 node=%s source=%s config=%s，默认不启用服务。\n' \
    "${node}" "${source_dir}" "${config_file}"
  exit 0
fi

install -d -m 0755 /opt/sub2api-ha-agent
cp -a "${source_dir}/sub2api_ha" /opt/sub2api-ha-agent/
install -d -m 0700 /etc/sub2api-ha /var/lib/sub2api-ha-agent
install -m 0600 "${config_file}" /etc/sub2api-ha/agent.json
install -d -m 0755 /usr/local/libexec
install -m 0755 "${source_dir}/bin/verify-action.sh" /usr/local/libexec/sub2api-ha-verify-action
install -m 0644 "${source_dir}/systemd/sub2api-ha-agent.service" /etc/systemd/system/sub2api-ha-agent.service
if [ "${node}" = "A" ]; then
  install -m 0755 "${source_dir}/bin/sync-release-if-needed.sh" /usr/local/libexec/sub2api-ha-sync-release-if-needed
  install -m 0644 "${source_dir}/systemd/sub2api-ha-release-sync.service" /etc/systemd/system/sub2api-ha-release-sync.service
  install -m 0644 "${source_dir}/systemd/sub2api-ha-release-sync.timer" /etc/systemd/system/sub2api-ha-release-sync.timer
fi
systemctl daemon-reload
printf '安装完成。请先执行 status/once 和观察模式验证，再单独启用服务。\n'
if [ "${node}" = "A" ]; then
  printf 'A 发布同步 timer 已安装但未启用，请在 Agent 稳定续租后单独启用。\n'
fi
