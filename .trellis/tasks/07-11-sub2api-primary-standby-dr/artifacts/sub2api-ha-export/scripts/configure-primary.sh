#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

ensure_replication_secret() {
  if [ ! -f "${SECRETS_FILE}" ]; then
    umask 077
    printf 'POSTGRES_REPLICATION_PASSWORD=%s\n' "$(openssl rand -hex 32)" > "${SECRETS_FILE}"
  fi
  chmod 0600 "${SECRETS_FILE}"
  # shellcheck disable=SC1090
  . "${SECRETS_FILE}"
  [[ "${POSTGRES_REPLICATION_PASSWORD:-}" =~ ^[a-f0-9]{64}$ ]] || die "复制密码文件格式无效"
}

validate_setting() {
  [[ "$2" =~ ^[1-9][0-9]*(MB|GB)$ ]] || die "$1 格式无效：$2"
}

main() {
  local pg_user hba_file network_subnet hba_line existing_slot_type

  require_command docker
  require_command openssl
  load_export_env
  require_var POSTGRES_REPLICATION_USER
  require_var POSTGRES_REPLICATION_SLOT
  require_var WAL_KEEP_SIZE
  require_var MAX_SLOT_WAL_KEEP_SIZE
  [[ "${POSTGRES_REPLICATION_USER}" =~ ^[a-z_][a-z0-9_]*$ ]] || die "复制用户名格式无效"
  [[ "${POSTGRES_REPLICATION_SLOT}" =~ ^[a-z_][a-z0-9_]*$ ]] || die "复制槽名称格式无效"
  validate_setting WAL_KEEP_SIZE "${WAL_KEEP_SIZE}"
  validate_setting MAX_SLOT_WAL_KEEP_SIZE "${MAX_SLOT_WAL_KEEP_SIZE}"
  ensure_replication_secret

  pg_user="$(postgres_user)"
  [ -n "${pg_user}" ] || die "无法识别 PostgreSQL管理员用户"
  hba_file="$(docker exec sub2api-postgres psql -U "${pg_user}" -d postgres -Atc "SHOW hba_file;")"
  network_subnet="$(docker network inspect deploy_sub2api-network --format '{{(index .IPAM.Config 0).Subnet}}')"
  [[ "${network_subnet}" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2}$ ]] || die "无法识别 A Docker网络子网"
  hba_line="host replication ${POSTGRES_REPLICATION_USER} ${network_subnet} scram-sha-256 # sub2api-primary-standby-dr"

  docker exec -i \
    -e REPLICATION_USER="${POSTGRES_REPLICATION_USER}" \
    -e REPLICATION_PASSWORD="${POSTGRES_REPLICATION_PASSWORD}" \
    sub2api-postgres sh -ec '
      psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres \
        -v repl_user="$REPLICATION_USER" \
        -v repl_password="$REPLICATION_PASSWORD"
    ' <<'SQL'
SELECT format('CREATE ROLE %I WITH LOGIN REPLICATION PASSWORD %L', :'repl_user', :'repl_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'repl_user')
\gexec
SELECT format('ALTER ROLE %I WITH LOGIN REPLICATION PASSWORD %L', :'repl_user', :'repl_password')
\gexec
SQL

  docker exec -u postgres \
    -e HBA_FILE="${hba_file}" \
    -e HBA_LINE="${hba_line}" \
    sub2api-postgres sh -ec '
      if grep -Fq "# sub2api-primary-standby-dr" "$HBA_FILE"; then
        sed -i "\\|# sub2api-primary-standby-dr$|c\\$HBA_LINE" "$HBA_FILE"
      else
        printf "\n%s\n" "$HBA_LINE" >> "$HBA_FILE"
      fi
    '

  docker exec sub2api-postgres psql -v ON_ERROR_STOP=1 -U "${pg_user}" -d postgres -c \
    "ALTER SYSTEM SET wal_keep_size = '${WAL_KEEP_SIZE}';"
  docker exec sub2api-postgres psql -v ON_ERROR_STOP=1 -U "${pg_user}" -d postgres -c \
    "ALTER SYSTEM SET max_slot_wal_keep_size = '${MAX_SLOT_WAL_KEEP_SIZE}';"

  existing_slot_type="$(docker exec sub2api-postgres psql -U "${pg_user}" -d postgres -Atc "SELECT slot_type FROM pg_replication_slots WHERE slot_name = '${POSTGRES_REPLICATION_SLOT}';")"
  [ -z "${existing_slot_type}" ] || [ "${existing_slot_type}" = "physical" ] || die "同名复制槽不是物理槽"
  if [ -z "${existing_slot_type}" ]; then
    docker exec sub2api-postgres psql -v ON_ERROR_STOP=1 -U "${pg_user}" -d postgres -c \
      "SELECT pg_create_physical_replication_slot('${POSTGRES_REPLICATION_SLOT}');"
  fi

  docker exec sub2api-postgres psql -v ON_ERROR_STOP=1 -U "${pg_user}" -d postgres -c 'SELECT pg_reload_conf();'
  "${SCRIPT_DIR}/verify-primary.sh"
  date -Is > "${STATE_DIR}/primary-configured"
  printf 'A PostgreSQL复制参数已在线配置，未执行数据库重启。\n'
}

main "$@"
