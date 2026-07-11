#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

postgres_container=absent
postgres_recovery=unknown
postgres_lsn=unknown
postgres_volume=unknown
redis_container=absent
redis_role=unknown
redis_link=unknown
redis_sync=unknown
redis_master_offset=unknown
redis_slave_offset=unknown
redis_volume=unknown
app_container=absent
app_volume=unknown
app_image_digest=absent
recovery_image_digest=unknown
recovery_image_cached=unknown
release_image_digest=unknown
release_source_ref=unknown
release_synced_at=unknown
current_mode=offline

redis_replication_info() {
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    docker exec -e REDISCLI_AUTH="${REDIS_PASSWORD}" sub2api-redis redis-cli INFO replication | tr -d '\r'
  else
    docker exec sub2api-redis sh -lc 'unset REDISCLI_AUTH; redis-cli INFO replication' | tr -d '\r'
  fi
}

mount_name() {
  local container="$1"
  local destination="$2"

  docker inspect --format \
    "{{range .Mounts}}{{if eq .Destination \"${destination}\"}}{{.Name}}{{end}}{{end}}" \
    "${container}" 2>/dev/null || true
}

load_state() {
  local replication recovery_layout=false

  postgres_container="$(container_state sub2api-postgres)"
  if [ "${postgres_container}" = "running" ]; then
    postgres_recovery="$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-postgres \
      psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atc 'SELECT pg_is_in_recovery();')"
    if [ "${postgres_recovery}" = "t" ]; then
      postgres_lsn="$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-postgres \
        psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atc "SELECT COALESCE(pg_last_wal_replay_lsn()::text, pg_last_wal_receive_lsn()::text, '0/0');")"
    elif [ "${postgres_recovery}" = "f" ]; then
      postgres_lsn="$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-postgres \
        psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atc 'SELECT pg_current_wal_lsn();')"
    fi
  fi
  postgres_volume="$(mount_name sub2api-postgres /var/lib/postgresql)"

  redis_container="$(container_state sub2api-redis)"
  if [ "${redis_container}" = "running" ]; then
    replication="$(redis_replication_info)"
    redis_role="$(printf '%s\n' "${replication}" | awk -F: '$1 == "role" { print $2 }')"
    redis_link="$(printf '%s\n' "${replication}" | awk -F: '$1 == "master_link_status" { print $2 }')"
    redis_sync="$(printf '%s\n' "${replication}" | awk -F: '$1 == "master_sync_in_progress" { print $2 }')"
    redis_master_offset="$(printf '%s\n' "${replication}" | awk -F: '$1 == "master_repl_offset" { print $2 }')"
    redis_slave_offset="$(printf '%s\n' "${replication}" | awk -F: '$1 == "slave_repl_offset" { print $2 }')"
  fi
  redis_volume="$(mount_name sub2api-redis /data)"

  app_container="$(container_state sub2api)"
  app_volume="$(mount_name sub2api /app/data)"
  if [ "${app_container}" = "running" ]; then
    app_image_digest="$(resolve_running_image_digest sub2api 2>/dev/null || printf 'unknown')"
  else
    app_image_digest=absent
  fi
  recovery_image_digest="$(env_value "${ENV_FILE}" SUB2API_IMAGE 2>/dev/null || printf 'unknown')"
  if validate_image_digest "${recovery_image_digest}" && image_is_cached "${recovery_image_digest}"; then
    recovery_image_cached=yes
  else
    recovery_image_cached=no
  fi
  release_image_digest="$(release_state_value APP_IMAGE_DIGEST 2>/dev/null || printf 'unknown')"
  release_source_ref="$(release_state_value SOURCE_IMAGE_REF 2>/dev/null || printf 'unknown')"
  release_synced_at="$(release_state_value SYNCED_AT 2>/dev/null || printf 'unknown')"

  if [ "${postgres_volume}" = "${A_RECOVERY_POSTGRES_VOLUME}" ] \
    && [ "${redis_volume}" = "${A_RECOVERY_REDIS_VOLUME}" ]; then
    recovery_layout=true
  fi

  if [ "${postgres_container}" != "running" ] \
    && [ "${redis_container}" != "running" ] \
    && [ "${app_container}" != "running" ]; then
    current_mode=offline
  elif [ "${recovery_layout}" = "true" ] \
    && [ "${postgres_recovery}" = "t" ] \
    && [ "${redis_role}" = "slave" ] \
    && [ "${redis_link}" = "up" ] \
    && [ "${redis_sync}" = "0" ] \
    && [ "${app_container}" != "running" ]; then
    current_mode=standby-from-b
  elif [ "${recovery_layout}" = "true" ] \
    && [ "${postgres_recovery}" = "f" ] \
    && [ "${redis_role}" = "slave" ] \
    && [ "${app_container}" != "running" ]; then
    current_mode=cutback-postgres-promoted
  elif [ "${recovery_layout}" = "true" ] \
    && [ "${postgres_recovery}" = "f" ] \
    && [ "${redis_role}" = "master" ] \
    && [ "${app_container}" = "running" ] \
    && [ "${app_volume}" = "${A_RECOVERY_APP_VOLUME}" ]; then
    current_mode=active-recovered
  elif [ "${recovery_layout}" = "true" ] \
    && [ "${postgres_recovery}" = "f" ] \
    && [ "${redis_role}" = "master" ] \
    && [ "${app_container}" != "running" ]; then
    current_mode=active-recovered-stopped
  elif [ "${postgres_recovery}" = "f" ] \
    && [ "${redis_role}" = "master" ] \
    && [ "${app_container}" = "running" ] \
    && [ "${recovery_layout}" != "true" ]; then
    current_mode=legacy-active
  else
    current_mode=inconsistent
  fi
}

print_state() {
  local machine="${1:-false}"

  if [ "${machine}" = "true" ]; then
    printf 'mode=%s\n' "${current_mode}"
    printf 'postgres_container=%s\n' "${postgres_container}"
    printf 'postgres_recovery=%s\n' "${postgres_recovery}"
    printf 'postgres_lsn=%s\n' "${postgres_lsn}"
    printf 'postgres_volume=%s\n' "${postgres_volume}"
    printf 'redis_container=%s\n' "${redis_container}"
    printf 'redis_role=%s\n' "${redis_role}"
    printf 'redis_link=%s\n' "${redis_link}"
    printf 'redis_sync=%s\n' "${redis_sync}"
    printf 'redis_master_offset=%s\n' "${redis_master_offset}"
    printf 'redis_slave_offset=%s\n' "${redis_slave_offset}"
    printf 'redis_volume=%s\n' "${redis_volume}"
    printf 'app_container=%s\n' "${app_container}"
    printf 'app_volume=%s\n' "${app_volume}"
    printf 'app_image_digest=%s\n' "${app_image_digest}"
    printf 'recovery_image_digest=%s\n' "${recovery_image_digest}"
    printf 'recovery_image_cached=%s\n' "${recovery_image_cached}"
    printf 'release_image_digest=%s\n' "${release_image_digest}"
    printf 'release_source_ref=%s\n' "${release_source_ref}"
    printf 'release_synced_at=%s\n' "${release_synced_at}"
    return 0
  fi

  printf 'A 模式：%s\n' "${current_mode}"
  printf 'PostgreSQL：container=%s recovery=%s lsn=%s volume=%s\n' \
    "${postgres_container}" "${postgres_recovery}" "${postgres_lsn}" "${postgres_volume}"
  printf 'Redis：container=%s role=%s link=%s sync=%s master_offset=%s slave_offset=%s volume=%s\n' \
    "${redis_container}" "${redis_role}" "${redis_link}" "${redis_sync}" \
    "${redis_master_offset}" "${redis_slave_offset}" "${redis_volume}"
  printf 'Sub2API：container=%s volume=%s\n' "${app_container}" "${app_volume}"
  printf '发布镜像：running=%s recovery=%s cached=%s\n' \
    "${app_image_digest}" "${recovery_image_digest}" "${recovery_image_cached}"
  printf '发布同步：digest=%s source=%s synced_at=%s\n' \
    "${release_image_digest}" "${release_source_ref}" "${release_synced_at}"
}

main() {
  local machine=false

  [ "$#" -le 1 ] || die "用法：$0 [--machine]"
  [ "${1:-}" != "--machine" ] || machine=true
  [ -z "${1:-}" ] || [ "${1}" = "--machine" ] || die "未知参数：${1}"

  require_command awk
  require_command docker
  load_recovery_env
  POSTGRES_USER="${POSTGRES_USER:-sub2api}"
  POSTGRES_DB="${POSTGRES_DB:-sub2api}"
  require_var POSTGRES_PASSWORD
  require_var A_RECOVERY_APP_VOLUME
  require_var A_RECOVERY_POSTGRES_VOLUME
  require_var A_RECOVERY_REDIS_VOLUME
  load_state
  print_state "${machine}"
  [ "${current_mode}" != "inconsistent" ] || return 2
}

main "$@"
