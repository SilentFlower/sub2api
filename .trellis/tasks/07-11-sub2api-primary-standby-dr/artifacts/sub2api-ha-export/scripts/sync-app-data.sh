#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

validate_image_reference() {
  [[ "$1" =~ ^[A-Za-z0-9._/@:+-]+$ ]] || die "辅助镜像引用包含非法字符"
}

local_volume_is_empty() {
  docker run --rm \
    --entrypoint /bin/sh \
    -v "${A_RECOVERY_APP_VOLUME}:/target" \
    "${POSTGRES_IMAGE}" \
    -ec '[ -z "$(find /target -mindepth 1 -maxdepth 1 -print -quit)" ]'
}

remote_mode() {
  ssh_b "cd '${B_DR_ROOT}' && ./scripts/switch-mode.sh status --machine" \
    | awk -F= '$1 == "mode" { print $2 }'
}

sync_from_b() {
  local dry_run="$1"
  local mode remote_command

  mode="$(remote_mode)"
  case "${mode}" in
    active|active-stopped) ;;
    *) die "B 当前模式为 ${mode}，不能作为 A 应用数据源" ;;
  esac

  if docker volume inspect "${A_RECOVERY_APP_VOLUME}" >/dev/null 2>&1; then
    local_volume_is_empty || die "A 恢复应用卷非空，拒绝覆盖"
  fi
  if [ "${dry_run}" = "true" ]; then
    printf 'dry-run：将从 B 的 sub2api-dr-app-data 同步全部非日志应用数据到 A 新恢复卷。\n'
    return 0
  fi

  recovery_volume_create "${A_RECOVERY_APP_VOLUME}" recovery-app-data
  local_volume_is_empty || die "A 恢复应用卷非空，拒绝覆盖"
  remote_command="docker run --rm --entrypoint /bin/sh -v sub2api-dr-app-data:/source '${POSTGRES_IMAGE}' -ec 'cd /source && [ -f .installed ] && [ -f config.yaml ] && tar --exclude=./logs -czf - .'"
  ssh_b "${remote_command}" \
    | docker run --rm -i \
      --entrypoint /bin/sh \
      -v "${A_RECOVERY_APP_VOLUME}:/target" \
      "${POSTGRES_IMAGE}" \
      -ec '
        tar -xzf - -C /target
        [ -f /target/.installed ]
        [ -f /target/config.yaml ]
        [ ! -e /target/logs ]
      '
  write_state a-app-data-synced-from-b
  printf 'B 的非日志应用数据已同步到 A 新恢复卷。\n'
}

sync_to_b() {
  local dry_run="$1"
  local mode remote_command

  mode="$(remote_mode)"
  [ "${mode}" = "standby" ] || die "B 当前模式为 ${mode}，只有重新入备且应用停止后才能回写应用数据"
  docker volume inspect "${A_RECOVERY_APP_VOLUME}" >/dev/null 2>&1 \
    || die "A 恢复应用卷不存在"

  if [ "${dry_run}" = "true" ]; then
    printf 'dry-run：将以 A 恢复应用卷为准，更新 B 容灾应用卷中的全部非日志数据。\n'
    return 0
  fi

  remote_command="docker run --rm -i --entrypoint /bin/sh -v sub2api-dr-app-data:/target '${POSTGRES_IMAGE}' -ec 'for path in /target/.[!.]* /target/..?* /target/*; do [ -e \"\$path\" ] || continue; [ \"\$path\" = /target/logs ] && continue; rm -rf -- \"\$path\"; done; tar -xzf - -C /target; [ -f /target/.installed ]; [ -f /target/config.yaml ]'"
  docker run --rm \
    --entrypoint /bin/sh \
    -v "${A_RECOVERY_APP_VOLUME}:/source:ro" \
    "${POSTGRES_IMAGE}" \
    -ec 'cd /source && [ -f .installed ] && [ -f config.yaml ] && tar --exclude=./logs -czf - .' \
    | ssh_b "${remote_command}"
  write_state b-app-data-synced-from-a
  printf 'A 的非日志应用数据已同步回 B 容灾应用卷。\n'
}

main() {
  local direction="${1:-}"
  local option="${2:-}"
  local dry_run=false

  [ "$#" -le 2 ] || die "用法：$0 <from-b|to-b> [--dry-run]"
  [ "${option}" != "--dry-run" ] || dry_run=true
  [ -z "${option}" ] || [ "${option}" = "--dry-run" ] || die "未知参数：${option}"

  require_command awk
  require_command docker
  require_command ssh
  load_recovery_env
  require_var POSTGRES_IMAGE
  require_var A_RECOVERY_APP_VOLUME
  validate_image_reference "${POSTGRES_IMAGE}"

  case "${direction}" in
    from-b) sync_from_b "${dry_run}" ;;
    to-b) sync_to_b "${dry_run}" ;;
    *) die "用法：$0 <from-b|to-b> [--dry-run]" ;;
  esac
}

main "$@"
