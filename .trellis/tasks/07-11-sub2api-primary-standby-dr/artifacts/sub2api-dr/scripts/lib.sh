#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
DR_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
STATE_DIR="${DR_ROOT}/state"
ENV_FILE="${DR_ROOT}/.env"

die() {
  printf '错误：%s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "缺少命令：$1"
}

load_runtime_env() {
  [ -f "${ENV_FILE}" ] || die "缺少 ${ENV_FILE}；请从 .env.example 生成真实 .env 后再执行"

  set -a
  # .env 由管理员维护，必须保持 KEY=VALUE 的 shell 兼容格式。
  # shellcheck disable=SC1090
  . "${ENV_FILE}"
  set +a

  mkdir -p "${STATE_DIR}"
}

require_var() {
  local name="$1"
  [ -n "${!name:-}" ] || die "环境变量 ${name} 不能为空"
  case "${!name}" in
    REPLACE_WITH_*) die "环境变量 ${name} 仍是占位值" ;;
  esac
}

compose_base() {
  docker compose \
    --project-name sub2api-dr \
    --env-file "${ENV_FILE}" \
    -f "${DR_ROOT}/compose.yaml" \
    "$@"
}

compose_promoted() {
  docker compose \
    --project-name sub2api-dr \
    --env-file "${ENV_FILE}" \
    -f "${DR_ROOT}/compose.yaml" \
    -f "${DR_ROOT}/compose.promoted.yaml" \
    "$@"
}

compose_recovery_export() {
  docker compose \
    --project-name sub2api-dr \
    --env-file "${ENV_FILE}" \
    -f "${DR_ROOT}/compose.yaml" \
    -f "${DR_ROOT}/compose.recovery-export.yaml" \
    "$@"
}

compose_volume_create() {
  local physical_name="$1"
  local logical_name="$2"
  local compose_version
  local actual_project
  local actual_volume

  compose_version="$(docker compose version --short)"
  docker volume create \
    --label com.docker.compose.project=sub2api-dr \
    --label "com.docker.compose.version=${compose_version}" \
    --label "com.docker.compose.volume=${logical_name}" \
    "${physical_name}" >/dev/null

  actual_project="$(docker volume inspect --format '{{index .Labels "com.docker.compose.project"}}' "${physical_name}")"
  actual_volume="$(docker volume inspect --format '{{index .Labels "com.docker.compose.volume"}}' "${physical_name}")"
  [ "${actual_project}" = "sub2api-dr" ] || die "卷 ${physical_name} 不属于 sub2api-dr Compose项目"
  [ "${actual_volume}" = "${logical_name}" ] || die "卷 ${physical_name} 的 Compose逻辑名称不匹配"
}

write_state() {
  local name="$1"
  date -Is > "${STATE_DIR}/${name}"
}

container_health() {
  docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$1"
}

wait_for_healthy() {
  local container="$1"
  local attempts="${2:-30}"
  local status

  for ((i = 1; i <= attempts; i++)); do
    status="$(container_health "${container}" 2>/dev/null || true)"
    if [ "${status}" = "healthy" ] || [ "${status}" = "running" ]; then
      return 0
    fi
    sleep 2
  done

  die "容器 ${container} 未在预期时间内就绪，当前状态：${status:-unknown}"
}
