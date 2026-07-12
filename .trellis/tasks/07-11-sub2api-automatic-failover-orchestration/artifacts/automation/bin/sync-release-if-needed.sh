#!/usr/bin/env bash

set -Eeuo pipefail

SWITCH_MODE_COMMAND="${SWITCH_MODE_COMMAND:-/root/sub2api-ha-export/scripts/switch-mode.sh}"
DOCKER_COMMAND="${DOCKER_COMMAND:-docker}"
LOCK_FILE="${LOCK_FILE:-/run/sub2api-ha-release-sync.lock}"
STABLE_SECONDS="${STABLE_SECONDS:-120}"

die() {
  printf '错误：%s\n' "$*" >&2
  exit 1
}

field() {
  local name="$1"
  awk -F= -v key="${name}" '$1 == key {sub(/^[^=]*=/, ""); print; found=1} END {if (!found) exit 1}'
}

[[ "${STABLE_SECONDS}" =~ ^[0-9]+$ ]] || die "STABLE_SECONDS 必须是非负整数"
[ -x "${SWITCH_MODE_COMMAND}" ] || die "switch-mode 入口不可执行：${SWITCH_MODE_COMMAND}"
command -v "${DOCKER_COMMAND}" >/dev/null 2>&1 || die "缺少 Docker 命令：${DOCKER_COMMAND}"
command -v flock >/dev/null 2>&1 || die "缺少 flock"

install -d -m 0755 "$(dirname -- "${LOCK_FILE}")"
exec 9>"${LOCK_FILE}"
if ! flock -n 9; then
  printf '发布镜像同步已有实例运行，本轮跳过。\n'
  exit 0
fi

status="$(${SWITCH_MODE_COMMAND} status --machine)"
mode="$(field mode <<<"${status}")" || die "状态缺少 mode"
case "${mode}" in
  legacy-active|active-recovered) ;;
  *) printf 'A 当前模式为 %s，不执行发布镜像同步。\n' "${mode}"; exit 0 ;;
esac

image_sync="$(field image_sync <<<"${status}")" || die "状态缺少 image_sync"
if [ "${image_sync}" = "ok" ]; then
  printf '发布镜像已经同步，无需处理。\n'
  exit 0
fi

app_container="$(field app_container <<<"${status}")" || die "状态缺少 app_container"
[ "${app_container}" = "running" ] || die "A 应用未运行，拒绝同步发布镜像"

started_at="$(${DOCKER_COMMAND} inspect --format '{{.State.StartedAt}}' sub2api)" \
  || die "无法读取 A 应用启动时间"
started_epoch="$(date -d "${started_at}" +%s)" || die "无法解析 A 应用启动时间：${started_at}"
now_epoch="$(date +%s)"
age_seconds="$((now_epoch - started_epoch))"
if [ "${age_seconds}" -lt "${STABLE_SECONDS}" ]; then
  printf 'A 新容器仅稳定 %s 秒，未达到 %s 秒，本轮跳过。\n' "${age_seconds}" "${STABLE_SECONDS}"
  exit 0
fi

printf '检测到发布镜像漂移，A 容器已稳定 %s 秒，开始门禁检查。\n' "${age_seconds}"
"${SWITCH_MODE_COMMAND}" sync-release --dry-run
"${SWITCH_MODE_COMMAND}" sync-release

final_status="$(${SWITCH_MODE_COMMAND} status --machine)"
final_sync="$(field image_sync <<<"${final_status}")" || die "同步后状态缺少 image_sync"
[ "${final_sync}" = "ok" ] || die "发布同步完成后 image_sync=${final_sync}"
printf '发布镜像自动调和完成，image_sync=ok。\n'
