#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

main() {
  require_command docker
  load_runtime_env
  require_var POSTGRES_USER
  require_var POSTGRES_PASSWORD
  require_var POSTGRES_DB

  docker container inspect sub2api-dr-postgres >/dev/null 2>&1 || die "PostgreSQL容灾容器不存在"
  docker container inspect sub2api-dr-redis >/dev/null 2>&1 || die "Redis容灾容器不存在"

  printf '%s\n' '--- PostgreSQL备库状态 ---'
  docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-dr-postgres \
    psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -x -c \
    "SELECT pg_is_in_recovery() AS in_recovery,
            pg_last_wal_receive_lsn() AS receive_lsn,
            pg_last_wal_replay_lsn() AS replay_lsn,
            pg_last_xact_replay_timestamp() AS last_replay_at,
            now() - pg_last_xact_replay_timestamp() AS replay_delay;
     SELECT status AS wal_receiver_status,
            slot_name,
            latest_end_lsn,
            last_msg_receipt_time
       FROM pg_stat_wal_receiver;"

  recovery="$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-dr-postgres psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atc 'SELECT pg_is_in_recovery();')"
  [ "${recovery}" = "t" ] || die "PostgreSQL当前不是备库"
  wal_receiver_status="$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-dr-postgres psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atc 'SELECT status FROM pg_stat_wal_receiver;')"
  [ "${wal_receiver_status}" = "streaming" ] || die "PostgreSQL WAL receiver 未处于 streaming 状态"

  printf '%s\n' '--- Redis从库状态 ---'
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    replication="$(docker exec -e REDISCLI_AUTH="${REDIS_PASSWORD}" sub2api-dr-redis redis-cli INFO replication | tr -d '\r')"
  else
    replication="$(docker exec sub2api-dr-redis sh -lc 'unset REDISCLI_AUTH; redis-cli INFO replication' | tr -d '\r')"
  fi
  printf '%s\n' "${replication}" | sed -n \
    '/^role:/p;/^master_link_status:/p;/^master_sync_in_progress:/p;/^master_last_io_seconds_ago:/p;/^slave_repl_offset:/p;/^master_repl_offset:/p'
  printf '%s\n' "${replication}" | grep -q '^role:slave$' || die "Redis当前不是从库"
  printf '%s\n' "${replication}" | grep -q '^master_link_status:up$' || die "Redis主链路未连接"
  printf '%s\n' "${replication}" | grep -q '^master_sync_in_progress:0$' || die "Redis初次同步或重同步尚未完成"

  printf 'PostgreSQL与 Redis复制状态检查通过。\n'
}

main "$@"
