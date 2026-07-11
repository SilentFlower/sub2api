#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

current_mode() {
  "${SCRIPT_DIR}/switch-mode.sh" status --machine | awk -F= '$1 == "mode" { print $2 }'
}

validate_identifier() {
  [[ "$2" =~ ^[a-z_][a-z0-9_]*$ ]] || die "$1 格式无效：$2"
}

port_available_or_owned() {
  local port="$1"
  local container="$2"

  if docker container inspect "${container}" >/dev/null 2>&1; then
    return 0
  fi
  ! ss -H -lnt | awk -v port=":${port}" '$4 ~ (port "$" ) { found = 1 } END { exit(found ? 0 : 1) }' \
    || die "宿主机端口 ${port} 已被其他进程占用"
}

verify_recovery_export_listeners() {
  local container port

  for container in sub2api-dr-postgres-recovery-export sub2api-dr-redis-recovery-export; do
    [ "$(docker inspect --format '{{.State.Status}}' "${container}")" = "running" ] \
      || die "临时恢复出口容器未运行：${container}"
  done
  for port in "${B_POSTGRES_RECOVERY_PORT}" "${B_REDIS_RECOVERY_PORT}"; do
    ss -H -lnt | awk -v port=":${port}" '$4 ~ (port "$" ) { found = 1 } END { exit(found ? 0 : 1) }' \
      || die "临时恢复出口端口未监听：${port}"
  done
}

configure_postgres_source() {
  local hba_file network_subnet hba_line existing_slot_type

  hba_file="$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-dr-postgres \
    psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atc 'SHOW hba_file;')"
  network_subnet="$(docker network inspect sub2api-dr-network --format '{{(index .IPAM.Config 0).Subnet}}')"
  [[ "${network_subnet}" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2}$ ]] \
    || die "无法识别 B 容灾网络子网"
  hba_line="host replication ${POSTGRES_REPLICATION_USER} ${network_subnet} scram-sha-256 # sub2api-a-recovery"

  docker exec -i \
    -e REPLICATION_USER="${POSTGRES_REPLICATION_USER}" \
    -e REPLICATION_PASSWORD="${POSTGRES_REPLICATION_PASSWORD}" \
    sub2api-dr-postgres sh -ec '
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
    sub2api-dr-postgres sh -ec '
      if grep -Fq "# sub2api-a-recovery" "$HBA_FILE"; then
        sed -i "\|# sub2api-a-recovery$|c\$HBA_LINE" "$HBA_FILE"
      else
        printf "\n%s\n" "$HBA_LINE" >> "$HBA_FILE"
      fi
    '

  existing_slot_type="$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-dr-postgres \
    psql -U "${POSTGRES_USER}" -d postgres -Atc \
    "SELECT slot_type FROM pg_replication_slots WHERE slot_name = '${POSTGRES_A_RECOVERY_SLOT}';")"
  [ -z "${existing_slot_type}" ] || [ "${existing_slot_type}" = "physical" ] \
    || die "同名 A 恢复复制槽不是物理槽"
  if [ -z "${existing_slot_type}" ]; then
    docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-dr-postgres \
      psql -v ON_ERROR_STOP=1 -U "${POSTGRES_USER}" -d postgres -c \
      "SELECT pg_create_physical_replication_slot('${POSTGRES_A_RECOVERY_SLOT}');"
  fi

  docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-dr-postgres \
    psql -v ON_ERROR_STOP=1 -U "${POSTGRES_USER}" -d postgres -c 'SELECT pg_reload_conf();' >/dev/null
}

main() {
  local dry_run=false
  local mode

  [ "$#" -le 1 ] || die "用法：$0 [--dry-run]"
  [ "${1:-}" != "--dry-run" ] || dry_run=true
  [ -z "${1:-}" ] || [ "${1}" = "--dry-run" ] || die "未知参数：${1}"

  require_command awk
  require_command docker
  require_command ss
  load_runtime_env
  require_var SOCAT_IMAGE
  require_var POSTGRES_USER
  require_var POSTGRES_PASSWORD
  require_var POSTGRES_DB
  require_var POSTGRES_REPLICATION_USER
  require_var POSTGRES_REPLICATION_PASSWORD
  require_var POSTGRES_A_RECOVERY_SLOT
  require_var A_SOURCE_CIDR
  require_var B_POSTGRES_RECOVERY_PORT
  require_var B_REDIS_RECOVERY_PORT
  validate_identifier POSTGRES_REPLICATION_USER "${POSTGRES_REPLICATION_USER}"
  validate_identifier POSTGRES_A_RECOVERY_SLOT "${POSTGRES_A_RECOVERY_SLOT}"

  mode="$(current_mode)"
  case "${mode}" in
    active|active-stopped) ;;
    *) die "B 当前模式为 ${mode}，只有已提升主库才能作为 A 的恢复源" ;;
  esac
  port_available_or_owned "${B_POSTGRES_RECOVERY_PORT}" sub2api-dr-postgres-recovery-export
  port_available_or_owned "${B_REDIS_RECOVERY_PORT}" sub2api-dr-redis-recovery-export

  if [ "${dry_run}" = "true" ]; then
    printf 'dry-run：将在线校准 A 恢复制用户/HBA/物理槽，并启动仅允许 A 来源的临时恢复出口。\n'
    printf 'dry-run 完成：B 数据库和应用均未修改。\n'
    return 0
  fi

  configure_postgres_source
  compose_recovery_export --profile recovery-source up -d \
    postgres-recovery-export redis-recovery-export
  wait_for_healthy sub2api-dr-postgres-recovery-export 30
  wait_for_healthy sub2api-dr-redis-recovery-export 30
  verify_recovery_export_listeners
  write_state recovery-source-ready
  printf 'B 临时恢复出口已监听；复制协议、物理槽和 Redis连通性将由 A 从允许的真实来源验证。\n'
}

main "$@"
