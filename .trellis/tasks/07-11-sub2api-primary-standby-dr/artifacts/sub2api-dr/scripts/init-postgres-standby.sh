#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

volume_is_empty() {
  docker run --rm \
    --entrypoint /bin/sh \
    -v sub2api-dr-postgres-data:/target \
    "${POSTGRES_IMAGE}" \
    -ec '[ -z "$(find /target -mindepth 1 -maxdepth 1 -print -quit)" ]'
}

main() {
  require_command docker
  load_runtime_env
  require_var POSTGRES_IMAGE
  require_var A_REPLICATION_HOST
  require_var A_POSTGRES_REPLICATION_PORT
  require_var POSTGRES_REPLICATION_USER
  require_var POSTGRES_REPLICATION_PASSWORD
  require_var POSTGRES_REPLICATION_SLOT
  require_var POSTGRES_USER
  require_var POSTGRES_PASSWORD
  require_var POSTGRES_DB

  if docker container inspect sub2api-dr-postgres >/dev/null 2>&1; then
    die "sub2api-dr-postgres 已存在；初始化要求目标容器不存在"
  fi

  compose_volume_create sub2api-dr-postgres-data postgres-data
  volume_is_empty || die "sub2api-dr-postgres-data 非空，拒绝覆盖"

  docker run --rm \
    --entrypoint /bin/sh \
    --mount type=volume,src=sub2api-dr-postgres-data,dst=/var/lib/postgresql,volume-nocopy \
    "${POSTGRES_IMAGE}" \
    -ec '
      mkdir -p /var/lib/postgresql/data
      chown -R postgres:postgres /var/lib/postgresql
    '

  docker run --rm \
    --name sub2api-dr-pg-basebackup \
    --network host \
    --user postgres \
    --entrypoint pg_basebackup \
    -e PGPASSWORD="${POSTGRES_REPLICATION_PASSWORD}" \
    --mount type=volume,src=sub2api-dr-postgres-data,dst=/var/lib/postgresql,volume-nocopy \
    "${POSTGRES_IMAGE}" \
    -h "${A_REPLICATION_HOST}" \
    -p "${A_POSTGRES_REPLICATION_PORT}" \
    -U "${POSTGRES_REPLICATION_USER}" \
    -D /var/lib/postgresql/data \
    -R \
    -X stream \
    -S "${POSTGRES_REPLICATION_SLOT}" \
    --checkpoint=spread \
    --progress

  docker run --rm \
    --entrypoint /bin/sh \
    -e REPLICATION_HOST="${A_REPLICATION_HOST}" \
    -e REPLICATION_PORT="${A_POSTGRES_REPLICATION_PORT}" \
    -e REPLICATION_USER="${POSTGRES_REPLICATION_USER}" \
    -e REPLICATION_PASSWORD="${POSTGRES_REPLICATION_PASSWORD}" \
    --mount type=volume,src=sub2api-dr-postgres-data,dst=/var/lib/postgresql,volume-nocopy \
    "${POSTGRES_IMAGE}" \
    -ec '
      escaped_password="$(printf "%s" "$REPLICATION_PASSWORD" | sed "s/\\\\/\\\\\\\\/g; s/:/\\\\:/g")"
      printf "%s:%s:*:%s:%s\n" "$REPLICATION_HOST" "$REPLICATION_PORT" "$REPLICATION_USER" "$escaped_password" > /var/lib/postgresql/data/.pgpass
      chmod 0600 /var/lib/postgresql/data/.pgpass
      chown postgres:postgres /var/lib/postgresql/data/.pgpass
      sed -i "/^primary_conninfo =/d" /var/lib/postgresql/data/postgresql.auto.conf
      printf "primary_conninfo = '\''host=%s port=%s user=%s passfile=/var/lib/postgresql/data/.pgpass application_name=sub2api-b'\''\n" "$REPLICATION_HOST" "$REPLICATION_PORT" "$REPLICATION_USER" >> /var/lib/postgresql/data/postgresql.auto.conf
      chown postgres:postgres /var/lib/postgresql/data/postgresql.auto.conf
    '

  compose_base --profile standby up -d --no-deps postgres-dr
  wait_for_healthy sub2api-dr-postgres 60

  recovery="$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-dr-postgres psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atc 'SELECT pg_is_in_recovery();')"
  [ "${recovery}" = "t" ] || die "PostgreSQL未处于备库恢复状态"
  write_state postgres-initialized
  printf 'PostgreSQL备库初始化完成。\n'
}

main "$@"
