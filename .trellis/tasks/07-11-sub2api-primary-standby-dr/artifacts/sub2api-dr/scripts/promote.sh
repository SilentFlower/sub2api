#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

postgres_is_primary() {
  [ "$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-dr-postgres psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atc 'SELECT pg_is_in_recovery();')" = "f" ]
}

redis_replication_info() {
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    docker exec -e REDISCLI_AUTH="${REDIS_PASSWORD}" sub2api-dr-redis redis-cli INFO replication | tr -d '\r'
  else
    docker exec sub2api-dr-redis sh -lc 'unset REDISCLI_AUTH; redis-cli INFO replication' | tr -d '\r'
  fi
}

redis_is_primary() {
  redis_replication_info | grep -q '^role:master$'
}

promote_redis_runtime() {
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    docker exec -e REDISCLI_AUTH="${REDIS_PASSWORD}" sub2api-dr-redis redis-cli REPLICAOF NO ONE >/dev/null
  else
    docker exec sub2api-dr-redis sh -lc 'unset REDISCLI_AUTH; redis-cli REPLICAOF NO ONE' >/dev/null
  fi
}

show_pre_promote_status() {
  printf '%s\n' '--- PostgreSQL最后接收与回放位置 ---'
  docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-dr-postgres \
    psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -x -c \
    "SELECT pg_is_in_recovery() AS in_recovery,
            pg_last_wal_receive_lsn() AS receive_lsn,
            pg_last_wal_replay_lsn() AS replay_lsn,
            pg_last_xact_replay_timestamp() AS last_replay_at,
            now() - pg_last_xact_replay_timestamp() AS replay_delay;"

  printf '%s\n' '--- Redis复制状态 ---'
  redis_replication_info
}

main() {
  local dry_run=false
  [ "${1:-}" = "--dry-run" ] && dry_run=true

  require_command docker
  require_command curl
  load_runtime_env
  require_var POSTGRES_USER
  require_var POSTGRES_PASSWORD
  require_var POSTGRES_DB

  docker inspect sub2api-dr-postgres >/dev/null 2>&1 || die "PostgreSQL容灾容器不存在"
  docker inspect sub2api-dr-redis >/dev/null 2>&1 || die "Redis容灾容器不存在"
  verify_release_image_ready
  show_pre_promote_status

  if [ "${dry_run}" = "true" ]; then
    printf 'dry-run 完成：没有提升数据库、重建容器或启动应用。\n'
    return 0
  fi

  printf '%s\n' '警告：没有自动 fencing。只有确认 A 已停止且不会继续写入时才能提升 B。'
  read -r -p '请输入 A_IS_STOPPED_AND_WILL_NOT_WRITE 继续：' confirmation
  [ "${confirmation}" = "A_IS_STOPPED_AND_WILL_NOT_WRITE" ] || die "确认口令不匹配，已取消"
  write_state operator-confirmed

  if [ -f "${STATE_DIR}/postgres-promoted" ]; then
    postgres_is_primary || die "已有 PostgreSQL提升标记，但数据库仍处于恢复状态"
  elif postgres_is_primary; then
    write_state postgres-promoted
  else
    docker exec -u postgres sub2api-dr-postgres \
      pg_ctl -D /var/lib/postgresql/data promote -w -t 60
    postgres_is_primary || die "PostgreSQL提升失败"
    write_state postgres-promoted
  fi

  if [ -f "${STATE_DIR}/redis-promoted" ]; then
    redis_is_primary || die "已有 Redis提升标记，但 Redis不是主库"
  else
    promote_redis_runtime
    redis_is_primary || die "Redis运行时提升失败"
    compose_promoted --profile promoted up -d --no-deps --force-recreate redis-dr
    wait_for_healthy sub2api-dr-redis 30
    redis_is_primary || die "Redis持久化主库配置验证失败"
    write_state redis-promoted
  fi

  if [ -f "${STATE_DIR}/app-started" ]; then
    [ "$(docker inspect --format '{{.State.Running}}' sub2api-dr-app 2>/dev/null || true)" = "true" ] || die "已有应用启动标记，但应用容器未运行"
  else
    compose_promoted --profile promoted up -d --no-deps app-dr
    wait_for_healthy sub2api-dr-app 60
    write_state app-started
  fi

  "${SCRIPT_DIR}/verify-service.sh"
  printf '%s\n' '数据库和应用已提升。公共入口尚未配置自动切换，请按阶段 4 的入口操作单人工切换。'
}

main "$@"
