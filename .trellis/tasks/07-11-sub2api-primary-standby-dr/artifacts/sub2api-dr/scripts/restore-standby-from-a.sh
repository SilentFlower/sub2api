#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

current_mode() {
  "${SCRIPT_DIR}/switch-mode.sh" status --machine | awk -F= '$1 == "mode" { print $2 }'
}

verify_a_source() {
  local replication_slot

  docker run --rm \
    --network host \
    --entrypoint pg_isready \
    "${POSTGRES_IMAGE}" \
    -h "${A_REPLICATION_HOST}" \
    -p "${A_POSTGRES_REPLICATION_PORT}" \
    -U "${POSTGRES_REPLICATION_USER}" >/dev/null

  docker run --rm \
    --network host \
    --entrypoint psql \
    -e PGPASSWORD="${POSTGRES_REPLICATION_PASSWORD}" \
    "${POSTGRES_IMAGE}" \
    "host=${A_REPLICATION_HOST} port=${A_POSTGRES_REPLICATION_PORT} user=${POSTGRES_REPLICATION_USER} dbname=postgres replication=database" \
    -Atc IDENTIFY_SYSTEM >/dev/null
  replication_slot="$(docker run --rm \
    --network host \
    --entrypoint psql \
    -e PGPASSWORD="${POSTGRES_REPLICATION_PASSWORD}" \
    "${POSTGRES_IMAGE}" \
    "host=${A_REPLICATION_HOST} port=${A_POSTGRES_REPLICATION_PORT} user=${POSTGRES_REPLICATION_USER} dbname=postgres replication=database" \
    -Atc "READ_REPLICATION_SLOT ${POSTGRES_REPLICATION_SLOT}")"
  [[ "${replication_slot}" == physical\|* ]] || die "A 上的 B 物理复制槽不可用"

  if [ -n "${REDIS_MASTER_PASSWORD:-}" ]; then
    docker run --rm \
      --network host \
      --entrypoint redis-cli \
      -e REDISCLI_AUTH="${REDIS_MASTER_PASSWORD}" \
      "${REDIS_IMAGE}" \
      -h "${A_REPLICATION_HOST}" \
      -p "${A_REDIS_REPLICATION_PORT}" ping | grep -qx PONG
  else
    docker run --rm \
      --network host \
      --entrypoint redis-cli \
      "${REDIS_IMAGE}" \
      -h "${A_REPLICATION_HOST}" \
      -p "${A_REDIS_REPLICATION_PORT}" ping | grep -qx PONG
  fi
}

remove_owned_volume() {
  local physical_name="$1"
  local logical_name="$2"
  local project_label volume_label attached

  docker volume inspect "${physical_name}" >/dev/null 2>&1 || return 0
  project_label="$(docker volume inspect --format '{{index .Labels "com.docker.compose.project"}}' "${physical_name}")"
  volume_label="$(docker volume inspect --format '{{index .Labels "com.docker.compose.volume"}}' "${physical_name}")"
  [ "${project_label}" = "sub2api-dr" ] || die "卷 ${physical_name} 不属于 sub2api-dr，拒绝删除"
  [ "${volume_label}" = "${logical_name}" ] || die "卷 ${physical_name} 的逻辑名称不匹配，拒绝删除"
  attached="$(docker ps -aq --filter "volume=${physical_name}")"
  [ -z "${attached}" ] || die "卷 ${physical_name} 仍被容器引用，拒绝删除"
  docker volume rm "${physical_name}" >/dev/null
}

clear_promoted_state() {
  rm -f \
    "${STATE_DIR}/operator-confirmed" \
    "${STATE_DIR}/postgres-promoted" \
    "${STATE_DIR}/redis-promoted" \
    "${STATE_DIR}/app-started" \
    "${STATE_DIR}/writes-frozen" \
    "${STATE_DIR}/recovery-source-ready"
}

main() {
  local dry_run=false
  local mode confirmation

  [ "$#" -le 1 ] || die "用法：$0 [--dry-run]"
  [ "${1:-}" != "--dry-run" ] || dry_run=true
  [ -z "${1:-}" ] || [ "${1}" = "--dry-run" ] || die "未知参数：${1}"

  require_command awk
  require_command docker
  require_command grep
  require_command ss
  load_runtime_env
  require_var POSTGRES_IMAGE
  require_var REDIS_IMAGE
  require_var A_REPLICATION_HOST
  require_var A_POSTGRES_REPLICATION_PORT
  require_var A_REDIS_REPLICATION_PORT
  require_var POSTGRES_REPLICATION_USER
  require_var POSTGRES_REPLICATION_PASSWORD
  require_var POSTGRES_REPLICATION_SLOT

  mode="$(current_mode)"
  [ "${mode}" = "active-stopped" ] \
    || die "B 当前模式为 ${mode}；只有应用已冻结且数据库仍为主库时才能重新入备"
  verify_a_source || die "新的 A 主节点复制出口不可用"

  if [ "${dry_run}" = "true" ]; then
    printf 'dry-run：将停止 B 容灾数据库，删除并重建 sub2api-dr PostgreSQL/Redis卷，再从新 A 主节点全量初始化。\n'
    printf 'dry-run 完成：没有停止容器、删除卷或改变复制方向。\n'
    return 0
  fi

  printf '%s\n' '警告：该操作会删除 B 容灾 PostgreSQL和 Redis当前数据卷，并从 A 重新做全量初始化。'
  read -r -p '请输入 REBUILD_B_STANDBY_FROM_A 继续：' confirmation
  [ "${confirmation}" = "REBUILD_B_STANDBY_FROM_A" ] || die "确认口令不匹配，已取消"

  compose_recovery_export --profile recovery-source down --remove-orphans
  compose_promoted --profile promoted stop app-dr postgres-dr redis-dr
  compose_promoted --profile promoted rm -f -s app-dr postgres-dr redis-dr
  remove_owned_volume sub2api-dr-postgres-data postgres-data
  remove_owned_volume sub2api-dr-redis-data redis-data
  clear_promoted_state
  write_state standby-rebuild-in-progress

  "${SCRIPT_DIR}/init-postgres-standby.sh"
  compose_base --profile standby up -d --no-deps redis-dr
  wait_for_healthy sub2api-dr-redis 60
  "${SCRIPT_DIR}/verify-replication.sh"

  [ "$(docker inspect --format '{{.State.Status}}' sub2api-dr-app 2>/dev/null || printf 'absent')" != "running" ] \
    || die "B 容灾应用不应在重新入备后运行"
  if ss -H -lnt | awk '$4 ~ /:18080$/ { found = 1 } END { exit(found ? 0 : 1) }'; then
    die "B 重新入备后 18080 仍被占用"
  fi

  rm -f "${STATE_DIR}/standby-rebuild-in-progress"
  write_state standby-rebuilt-from-a
  printf 'B 已从新的 A 主节点重新初始化为备库，容灾应用保持停止。\n'
}

main "$@"
