#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

main() {
  require_command docker
  require_command curl
  load_runtime_env
  require_var POSTGRES_USER
  require_var POSTGRES_PASSWORD
  require_var POSTGRES_DB

  recovery="$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-dr-postgres psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atc 'SELECT pg_is_in_recovery();')"
  [ "${recovery}" = "f" ] || die "PostgreSQL尚未提升为主库"

  if [ -n "${REDIS_PASSWORD:-}" ]; then
    replication="$(docker exec -e REDISCLI_AUTH="${REDIS_PASSWORD}" sub2api-dr-redis redis-cli INFO replication | tr -d '\r')"
  else
    replication="$(docker exec sub2api-dr-redis sh -lc 'unset REDISCLI_AUTH; redis-cli INFO replication' | tr -d '\r')"
  fi
  role="$(printf '%s\n' "${replication}" | awk -F: '$1 == "role" { print $2 }')"
  [ "${role}" = "master" ] || die "Redis尚未提升为主库"

  [ "$(docker inspect --format '{{.State.Running}}' sub2api-dr-app 2>/dev/null || true)" = "true" ] || die "备用应用未运行"
  curl -fsS "http://127.0.0.1:${DR_APP_PORT:-18080}/health" >/dev/null || die "备用应用健康检查失败"
  printf 'B 容灾应用健康检查通过：127.0.0.1:%s/health\n' "${DR_APP_PORT:-18080}"
}

main "$@"
