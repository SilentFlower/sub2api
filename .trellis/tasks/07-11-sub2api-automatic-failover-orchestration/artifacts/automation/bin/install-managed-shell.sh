#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ARTIFACT_ROOT="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"

usage() {
  cat <<'EOF'
用法：
  install-managed-shell.sh --node A|B [--target-root <目录>] [--dry-run]

说明：
  只安装当前自动容灾任务受管的 switch-mode.sh，不复制或修改其它容灾文件。
  A 默认目标为 /root/sub2api-ha-export，B 默认目标为 /root/sub2api-dr。
  安装前会校验目标已有 lib.sh，并为旧入口创建带 UTC 时间戳的备份。
EOF
}

die() {
  printf '错误：%s\n' "$*" >&2
  exit 1
}

node=""
target_root=""
dry_run=false

while [ "$#" -gt 0 ]; do
  case "$1" in
    --node) node="${2:-}"; shift 2 ;;
    --target-root) target_root="${2:-}"; shift 2 ;;
    --dry-run) dry_run=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知参数：$1" ;;
  esac
done

case "${node}" in
  A)
    source_file="${ARTIFACT_ROOT}/managed-shell/sub2api-ha-export/switch-mode.sh"
    target_root="${target_root:-/root/sub2api-ha-export}"
    ;;
  B)
    source_file="${ARTIFACT_ROOT}/managed-shell/sub2api-dr/switch-mode.sh"
    target_root="${target_root:-/root/sub2api-dr}"
    ;;
  *) die "--node 必须是 A 或 B" ;;
esac

target_file="${target_root}/scripts/switch-mode.sh"
[ -f "${source_file}" ] || die "受管入口不存在：${source_file}"
[ -d "${target_root}/scripts" ] || die "目标容灾脚本目录不存在：${target_root}/scripts"
[ -f "${target_root}/scripts/lib.sh" ] || die "目标缺少既有 lib.sh，拒绝安装到错误目录"
bash -n "${source_file}"

if [ -f "${target_file}" ] && cmp -s "${source_file}" "${target_file}"; then
  printf '目标已是最新受管版本：%s\n' "${target_file}"
  exit 0
fi

if [ "${dry_run}" = "true" ]; then
  printf 'dry-run：将把 %s 安装到 %s。\n' "${source_file}" "${target_file}"
  [ ! -f "${target_file}" ] || printf 'dry-run：将先备份当前入口，但不会修改任何文件。\n'
  exit 0
fi

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
if [ -f "${target_file}" ]; then
  cp -a -- "${target_file}" "${target_file}.pre-ha-auto.${timestamp}"
fi
temporary="${target_file}.tmp.$$"
trap 'rm -f -- "${temporary}"' EXIT
install -m 0755 "${source_file}" "${temporary}"
mv -f -- "${temporary}" "${target_file}"
trap - EXIT

printf '已安装节点 %s 的受管入口：%s\n' "${node}" "${target_file}"
printf '下一步只运行 status --machine 和相关 --dry-run；不要直接执行生产提升。\n'

