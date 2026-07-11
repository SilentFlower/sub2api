#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

POSTGRES_CONTAINER=sub2api-dr-postgres
REDIS_CONTAINER=sub2api-dr-redis
APP_CONTAINER=sub2api-dr-app

postgres_container_state=absent
postgres_recovery=unknown
redis_container_state=absent
redis_role=unknown
redis_link=unknown
redis_sync=unknown
postgres_lsn=unknown
redis_master_offset=unknown
redis_slave_offset=unknown
app_container_state=absent
app_image_digest=unknown
app_image_cached=unknown
running_app_image_digest=absent
release_image_digest=unknown
release_source_ref=unknown
release_synced_at=unknown
current_mode=uninitialized

usage() {
  cat <<'EOF'
用法：
  switch-mode.sh status
  switch-mode.sh standby [--dry-run]
  switch-mode.sh enable [--dry-run]
  switch-mode.sh freeze [--dry-run]

说明：
  status   只读显示当前模式和各组件角色。
  standby  仅在数据库仍是备库/从库时停止备用应用并复核复制。
  enable   调用现有提升流程，仍需人工确认 A 已停止且不会继续写入。
  freeze   B 已启用时只停止容灾应用，数据库继续保持主库供 A 追平。

已提升并产生写入的 B 不能通过 standby 原地降回备库，必须重新同步。
EOF
}

redis_replication_info() {
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    docker exec -e REDISCLI_AUTH="${REDIS_PASSWORD}" "${REDIS_CONTAINER}" redis-cli INFO replication | tr -d '\r'
  else
    docker exec "${REDIS_CONTAINER}" sh -lc 'unset REDISCLI_AUTH; redis-cli INFO replication' | tr -d '\r'
  fi
}

inspect_container_state() {
  if docker container inspect "$1" >/dev/null 2>&1; then
    docker inspect --format '{{.State.Status}}' "$1"
  else
    printf 'absent\n'
  fi
}

load_current_state() {
  local replication

  postgres_container_state="$(inspect_container_state "${POSTGRES_CONTAINER}")"
  postgres_recovery=unknown
  if [ "${postgres_container_state}" = "running" ]; then
    postgres_recovery="$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" "${POSTGRES_CONTAINER}" \
      psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atc 'SELECT pg_is_in_recovery();')"
    if [ "${postgres_recovery}" = "t" ]; then
      postgres_lsn="$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" "${POSTGRES_CONTAINER}" \
        psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atc "SELECT COALESCE(pg_last_wal_replay_lsn()::text, pg_last_wal_receive_lsn()::text, '0/0');")"
    elif [ "${postgres_recovery}" = "f" ]; then
      postgres_lsn="$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" "${POSTGRES_CONTAINER}" \
        psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atc 'SELECT pg_current_wal_lsn();')"
    fi
  fi

  redis_container_state="$(inspect_container_state "${REDIS_CONTAINER}")"
  redis_role=unknown
  redis_link=unknown
  redis_sync=unknown
  if [ "${redis_container_state}" = "running" ]; then
    replication="$(redis_replication_info)"
    redis_role="$(printf '%s\n' "${replication}" | awk -F: '$1 == "role" { print $2 }')"
    redis_link="$(printf '%s\n' "${replication}" | awk -F: '$1 == "master_link_status" { print $2 }')"
    redis_sync="$(printf '%s\n' "${replication}" | awk -F: '$1 == "master_sync_in_progress" { print $2 }')"
    redis_master_offset="$(printf '%s\n' "${replication}" | awk -F: '$1 == "master_repl_offset" { print $2 }')"
    redis_slave_offset="$(printf '%s\n' "${replication}" | awk -F: '$1 == "slave_repl_offset" { print $2 }')"
  fi

  app_container_state="$(inspect_container_state "${APP_CONTAINER}")"
  app_image_digest="$(env_value "${ENV_FILE}" SUB2API_IMAGE 2>/dev/null || printf 'unknown')"
  if validate_image_digest "${app_image_digest}" && image_is_cached "${app_image_digest}"; then
    app_image_cached=yes
  else
    app_image_cached=no
  fi
  if [ "${app_container_state}" = "running" ]; then
    running_app_image_digest="$(docker inspect --format '{{.Config.Image}}' "${APP_CONTAINER}" 2>/dev/null || printf 'unknown')"
  else
    running_app_image_digest=absent
  fi
  release_image_digest="$(release_state_value APP_IMAGE_DIGEST 2>/dev/null || printf 'unknown')"
  release_source_ref="$(release_state_value SOURCE_IMAGE_REF 2>/dev/null || printf 'unknown')"
  release_synced_at="$(release_state_value SYNCED_AT 2>/dev/null || printf 'unknown')"

  if [ "${postgres_recovery}" = "t" ] \
    && [ "${redis_role}" = "slave" ] \
    && [ "${redis_link}" = "up" ] \
    && [ "${redis_sync}" = "0" ] \
    && [ "${app_container_state}" != "running" ]; then
    current_mode=standby
  elif [ "${postgres_recovery}" = "f" ] \
    && [ "${redis_role}" = "master" ] \
    && [ "${app_container_state}" = "running" ]; then
    current_mode=active
  elif [ "${postgres_recovery}" = "f" ] \
    && [ "${redis_role}" = "master" ] \
    && [ "${app_container_state}" != "running" ]; then
    current_mode=active-stopped
  elif [ "${postgres_container_state}" = "absent" ] \
    && [ "${redis_container_state}" = "absent" ]; then
    current_mode=uninitialized
  else
    current_mode=inconsistent
  fi
}

print_current_state() {
  local machine="${1:-false}"

  if [ "${machine}" = "true" ]; then
    printf 'mode=%s\n' "${current_mode}"
    printf 'postgres_container=%s\n' "${postgres_container_state}"
    printf 'postgres_recovery=%s\n' "${postgres_recovery}"
    printf 'postgres_lsn=%s\n' "${postgres_lsn}"
    printf 'redis_container=%s\n' "${redis_container_state}"
    printf 'redis_role=%s\n' "${redis_role}"
    printf 'redis_link=%s\n' "${redis_link}"
    printf 'redis_sync=%s\n' "${redis_sync}"
    printf 'redis_master_offset=%s\n' "${redis_master_offset}"
    printf 'redis_slave_offset=%s\n' "${redis_slave_offset}"
    printf 'app_container=%s\n' "${app_container_state}"
    printf 'app_image_digest=%s\n' "${app_image_digest}"
    printf 'app_image_cached=%s\n' "${app_image_cached}"
    printf 'running_app_image_digest=%s\n' "${running_app_image_digest}"
    printf 'release_image_digest=%s\n' "${release_image_digest}"
    printf 'release_source_ref=%s\n' "${release_source_ref}"
    printf 'release_synced_at=%s\n' "${release_synced_at}"
    return 0
  fi

  printf '模式：%s\n' "${current_mode}"
  printf 'PostgreSQL：container=%s recovery=%s\n' "${postgres_container_state}" "${postgres_recovery}"
  printf 'PostgreSQL LSN：%s\n' "${postgres_lsn}"
  printf 'Redis：container=%s role=%s link=%s sync_in_progress=%s\n' \
    "${redis_container_state}" "${redis_role}" "${redis_link}" "${redis_sync}"
  printf 'Redis offset：master=%s slave=%s\n' "${redis_master_offset}" "${redis_slave_offset}"
  printf 'Sub2API：container=%s\n' "${app_container_state}"
  printf '发布镜像：configured=%s cached=%s running=%s\n' \
    "${app_image_digest}" "${app_image_cached}" "${running_app_image_digest}"
  printf '发布同步：digest=%s source=%s synced_at=%s\n' \
    "${release_image_digest}" "${release_source_ref}" "${release_synced_at}"
}

port_18080_is_listening() {
  ss -H -lnt | awk '$4 ~ /:18080$/ { found = 1 } END { exit(found ? 0 : 1) }'
}

warn_unsynchronized_clock() {
  local synchronized

  command -v timedatectl >/dev/null 2>&1 || return 0
  synchronized="$(timedatectl show -p NTPSynchronized --value 2>/dev/null || true)"
  if [ "${synchronized}" != "yes" ]; then
    printf '%s\n' '警告：B 当前未报告 NTP同步，提升前应确认系统时间。' >&2
  fi
}

ensure_standby() {
  local dry_run="$1"

  load_current_state
  print_current_state

  if [ "${postgres_recovery}" = "f" ]; then
    die "B PostgreSQL已是主库，不能原地切回 standby；必须从新的主库重新初始化"
  fi
  [ "${postgres_recovery}" = "t" ] || die "B PostgreSQL不是可用备库"
  [ "${redis_role}" = "slave" ] || die "B Redis不是从库，拒绝标记为 standby"
  [ "${redis_link}" = "up" ] || die "B Redis主链路未连接"
  [ "${redis_sync}" = "0" ] || die "B Redis仍在执行同步"

  if [ "${dry_run}" = "true" ]; then
    if [ "${app_container_state}" = "running" ]; then
      printf 'dry-run：将停止 %s。\n' "${APP_CONTAINER}"
    fi
    "${SCRIPT_DIR}/verify-replication.sh" >/dev/null
    printf 'dry-run 完成：当前角色允许收敛到 standby，没有停止容器或修改状态。\n'
    return 0
  fi

  if [ "${app_container_state}" = "running" ]; then
    compose_base --profile promoted stop app-dr
  fi
  [ "$(inspect_container_state "${APP_CONTAINER}")" != "running" ] || die "备用应用停止失败"
  port_18080_is_listening && die "备用应用停止后 18080 仍被占用"

  "${SCRIPT_DIR}/verify-replication.sh"
  write_state standby-verified
  load_current_state
  [ "${current_mode}" = "standby" ] || die "组件状态未收敛到 standby"
  printf '已进入 standby：数据库持续复制，备用应用停止。\n'
}

enable_mode() {
  local dry_run="$1"

  load_current_state
  print_current_state
  verify_release_image_ready
  warn_unsynchronized_clock

  if [ "${dry_run}" = "true" ]; then
    "${SCRIPT_DIR}/promote.sh" --dry-run
    printf 'enable dry-run 完成：没有提升数据库或启动应用。\n'
    return 0
  fi

  "${SCRIPT_DIR}/promote.sh"
  load_current_state
  [ "${current_mode}" = "active" ] || die "提升流程结束后未识别为 active"
  printf '已进入 active：数据库为主库，备用应用已启动。\n'
}

freeze_mode() {
  local dry_run="$1"
  local confirmation

  load_current_state
  print_current_state

  [ "${postgres_recovery}" = "f" ] || die "B PostgreSQL不是已提升主库，不能执行 freeze"
  [ "${redis_role}" = "master" ] || die "B Redis不是主库，不能执行 freeze"
  case "${current_mode}" in
    active|active-stopped) ;;
    *) die "当前模式 ${current_mode} 不允许执行 freeze" ;;
  esac

  if [ "${dry_run}" = "true" ]; then
    if [ "${app_container_state}" = "running" ]; then
      printf 'dry-run：将停止 %s，并保持 PostgreSQL和 Redis主库角色不变。\n' "${APP_CONTAINER}"
    else
      printf 'dry-run：B 应用已停止，将只复核数据库主库角色和 18080 监听状态。\n'
    fi
    printf 'freeze dry-run 完成：没有停止容器或改变数据库角色。\n'
    return 0
  fi

  if [ "${app_container_state}" = "running" ]; then
    printf '%s\n' '警告：freeze 会停止 B 容灾应用写入，但 PostgreSQL和 Redis继续保持主库。'
    read -r -p '请输入 FREEZE_B_WRITES_FOR_CUTBACK 继续：' confirmation
    [ "${confirmation}" = "FREEZE_B_WRITES_FOR_CUTBACK" ] || die "确认口令不匹配，已取消"
    compose_promoted --profile promoted stop app-dr
  fi

  [ "$(inspect_container_state "${APP_CONTAINER}")" != "running" ] || die "B 容灾应用停止失败"
  port_18080_is_listening && die "B 容灾应用停止后 18080 仍被占用"
  load_current_state
  [ "${current_mode}" = "active-stopped" ] || die "freeze 后组件状态不是 active-stopped"
  [ "${postgres_recovery}" = "f" ] || die "freeze 意外改变了 PostgreSQL角色"
  [ "${redis_role}" = "master" ] || die "freeze 意外改变了 Redis角色"
  write_state writes-frozen
  printf 'B 写入已冻结：应用停止，PostgreSQL和 Redis仍为主库。\n'
}

main() {
  local action="${1:-status}"
  local option="${2:-}"
  local dry_run=false
  local machine=false

  [ "$#" -le 2 ] || die "参数过多；请使用 --help 查看用法"

  case "${action}" in
    -h|--help|help)
      [ -z "${option}" ] || die "帮助命令不接受额外参数"
      usage
      return 0
      ;;
  esac

  case "${action}:${option}" in
    status:) ;;
    status:--machine) machine=true ;;
    standby:|enable:|freeze:) ;;
    standby:--dry-run|enable:--dry-run|freeze:--dry-run) dry_run=true ;;
    *) die "操作 ${action} 不支持参数：${option:-<空>}" ;;
  esac

  require_command awk
  require_command docker
  require_command ss
  load_runtime_env
  require_var POSTGRES_USER
  require_var POSTGRES_PASSWORD
  require_var POSTGRES_DB

  case "${action}" in
    status)
      load_current_state
      print_current_state "${machine}"
      [ "${current_mode}" != "inconsistent" ] || return 2
      ;;
    standby)
      ensure_standby "${dry_run}"
      ;;
    enable)
      enable_mode "${dry_run}"
      ;;
    freeze)
      freeze_mode "${dry_run}"
      ;;
    *)
      usage >&2
      die "未知操作：${action}"
      ;;
  esac
}

main "$@"
