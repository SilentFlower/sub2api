#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

main() {
  local pg_user

  require_command docker
  load_export_env
  require_var POSTGRES_REPLICATION_USER
  require_var POSTGRES_REPLICATION_SLOT
  pg_user="$(postgres_user)"
  [ -n "${pg_user}" ] || die "无法识别 PostgreSQL管理员用户"

  for container in sub2api-ha-postgres-export sub2api-ha-redis-export; do
    [ "$(docker inspect --format '{{.State.Running}}' "${container}" 2>/dev/null || true)" = "true" ] || die "转发容器未运行：${container}"
  done

  docker exec sub2api-postgres psql -v ON_ERROR_STOP=1 -U "${pg_user}" -d postgres -x -c \
    "SELECT rolname, rolcanlogin, rolreplication FROM pg_roles WHERE rolname = '${POSTGRES_REPLICATION_USER}';"
  docker exec sub2api-postgres psql -v ON_ERROR_STOP=1 -U "${pg_user}" -d postgres -x -c \
    "SELECT slot_name, slot_type, active, restart_lsn, wal_status FROM pg_replication_slots WHERE slot_name = '${POSTGRES_REPLICATION_SLOT}';"
  docker exec sub2api-postgres psql -v ON_ERROR_STOP=1 -U "${pg_user}" -d postgres -x -c \
    "SELECT name, setting, unit, context, pending_restart FROM pg_settings WHERE name IN ('wal_keep_size','max_slot_wal_keep_size') ORDER BY name;"
  docker exec sub2api-postgres psql -v ON_ERROR_STOP=1 -U "${pg_user}" -d postgres -x -c \
    "SELECT line_number, type, database, user_name, address, auth_method, error FROM pg_hba_file_rules WHERE '${POSTGRES_REPLICATION_USER}' = ANY(user_name);"

  role_ok="$(docker exec sub2api-postgres psql -U "${pg_user}" -d postgres -Atc "SELECT rolcanlogin AND rolreplication FROM pg_roles WHERE rolname = '${POSTGRES_REPLICATION_USER}';")"
  [ "${role_ok}" = "t" ] || die "复制用户不存在或权限不正确"
  slot_ok="$(docker exec sub2api-postgres psql -U "${pg_user}" -d postgres -Atc "SELECT slot_type = 'physical' FROM pg_replication_slots WHERE slot_name = '${POSTGRES_REPLICATION_SLOT}';")"
  [ "${slot_ok}" = "t" ] || die "物理复制槽不存在"
  hba_error_count="$(docker exec sub2api-postgres psql -U "${pg_user}" -d postgres -Atc "SELECT count(*) FROM pg_hba_file_rules WHERE error IS NOT NULL;")"
  [ "${hba_error_count}" = "0" ] || die "pg_hba.conf 存在解析错误"
  pending_restart_count="$(docker exec sub2api-postgres psql -U "${pg_user}" -d postgres -Atc "SELECT count(*) FROM pg_settings WHERE name IN ('wal_keep_size','max_slot_wal_keep_size') AND pending_restart;")"
  [ "${pending_restart_count}" = "0" ] || die "复制参数仍需要重启"
  printf 'A PostgreSQL复制用户、HBA、参数和物理槽验证通过。\n'
}

main "$@"
