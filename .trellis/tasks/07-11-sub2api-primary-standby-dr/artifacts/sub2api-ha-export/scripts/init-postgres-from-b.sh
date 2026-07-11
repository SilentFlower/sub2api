#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

volume_is_empty() {
  docker run --rm \
    --entrypoint /bin/sh \
    --mount "type=volume,src=${A_RECOVERY_POSTGRES_VOLUME},dst=/var/lib/postgresql,volume-nocopy" \
    "${POSTGRES_IMAGE}" \
    -ec '[ -z "$(find /var/lib/postgresql -mindepth 1 -maxdepth 1 -print -quit)" ]'
}

main() {
  local recovery wal_receiver_status

  require_command docker
  require_command grep
  load_recovery_env
  POSTGRES_USER="${POSTGRES_USER:-sub2api}"
  POSTGRES_DB="${POSTGRES_DB:-sub2api}"
  require_var POSTGRES_IMAGE
  require_var POSTGRES_PASSWORD
  require_var POSTGRES_REPLICATION_USER
  require_var POSTGRES_REPLICATION_PASSWORD
  require_var POSTGRES_A_RECOVERY_SLOT
  require_var B_REPLICATION_HOST
  require_var B_POSTGRES_RECOVERY_PORT
  require_var A_RECOVERY_POSTGRES_VOLUME

  docker image inspect --format '{{json .Config.Volumes}}' "${POSTGRES_IMAGE}" \
    | grep -q '"/var/lib/postgresql"' \
    || die "PostgreSQL镜像未声明预期父级卷 /var/lib/postgresql"
  if docker container inspect sub2api-postgres >/dev/null 2>&1; then
    die "sub2api-postgres 仍存在；初始化要求旧容器已停止并移除"
  fi

  recovery_volume_create "${A_RECOVERY_POSTGRES_VOLUME}" recovery-postgres-data
  volume_is_empty || die "A 恢复 PostgreSQL卷非空，拒绝覆盖"

  docker run --rm \
    --entrypoint /bin/sh \
    --mount "type=volume,src=${A_RECOVERY_POSTGRES_VOLUME},dst=/var/lib/postgresql,volume-nocopy" \
    "${POSTGRES_IMAGE}" \
    -ec '
      mkdir -p /var/lib/postgresql/data
      chown -R postgres:postgres /var/lib/postgresql
    '

  docker run --rm \
    --name sub2api-ha-pg-basebackup \
    --network host \
    --user postgres \
    --entrypoint pg_basebackup \
    -e PGPASSWORD="${POSTGRES_REPLICATION_PASSWORD}" \
    --mount "type=volume,src=${A_RECOVERY_POSTGRES_VOLUME},dst=/var/lib/postgresql,volume-nocopy" \
    "${POSTGRES_IMAGE}" \
    -h "${B_REPLICATION_HOST}" \
    -p "${B_POSTGRES_RECOVERY_PORT}" \
    -U "${POSTGRES_REPLICATION_USER}" \
    -D /var/lib/postgresql/data \
    -R \
    -X stream \
    -S "${POSTGRES_A_RECOVERY_SLOT}" \
    --checkpoint=spread \
    --progress

  docker run --rm \
    --entrypoint /bin/sh \
    -e REPLICATION_HOST="${B_REPLICATION_HOST}" \
    -e REPLICATION_PORT="${B_POSTGRES_RECOVERY_PORT}" \
    -e REPLICATION_USER="${POSTGRES_REPLICATION_USER}" \
    -e REPLICATION_PASSWORD="${POSTGRES_REPLICATION_PASSWORD}" \
    --mount "type=volume,src=${A_RECOVERY_POSTGRES_VOLUME},dst=/var/lib/postgresql,volume-nocopy" \
    "${POSTGRES_IMAGE}" \
    -ec '
      escaped_password="$(printf "%s" "$REPLICATION_PASSWORD" | sed "s/\\\\/\\\\\\\\/g; s/:/\\\\:/g")"
      printf "%s:%s:*:%s:%s\n" "$REPLICATION_HOST" "$REPLICATION_PORT" "$REPLICATION_USER" "$escaped_password" > /var/lib/postgresql/data/.pgpass
      chmod 0600 /var/lib/postgresql/data/.pgpass
      chown postgres:postgres /var/lib/postgresql/data/.pgpass
      sed -i "/^primary_conninfo =/d" /var/lib/postgresql/data/postgresql.auto.conf
      printf "primary_conninfo = '\''host=%s port=%s user=%s passfile=/var/lib/postgresql/data/.pgpass application_name=sub2api-a-recovery'\''\n" "$REPLICATION_HOST" "$REPLICATION_PORT" "$REPLICATION_USER" >> /var/lib/postgresql/data/postgresql.auto.conf
      chown postgres:postgres /var/lib/postgresql/data/postgresql.auto.conf
    '

  compose_recovery --profile cutback up -d --no-deps postgres
  wait_for_healthy sub2api-postgres 60
  recovery="$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-postgres \
    psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atc 'SELECT pg_is_in_recovery();')"
  [ "${recovery}" = "t" ] || die "A PostgreSQL未处于备库恢复状态"
  wal_receiver_status="$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-postgres \
    psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atc 'SELECT status FROM pg_stat_wal_receiver;')"
  [ "${wal_receiver_status}" = "streaming" ] || die "A PostgreSQL WAL receiver 未处于 streaming 状态"
  write_state a-postgres-standby-from-b
  printf 'A PostgreSQL已使用新命名卷从 B 初始化为备库。\n'
}

main "$@"
