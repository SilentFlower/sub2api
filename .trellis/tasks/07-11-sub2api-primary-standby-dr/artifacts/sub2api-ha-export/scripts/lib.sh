#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
EXPORT_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
STATE_DIR="${EXPORT_ROOT}/state"
ENV_FILE="${EXPORT_ROOT}/.env"
# 该变量由加载本文件的配置脚本使用。
# shellcheck disable=SC2034
SECRETS_FILE="${EXPORT_ROOT}/secrets.env"

die() {
  printf '错误：%s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "缺少命令：$1"
}

load_export_env() {
  [ -f "${ENV_FILE}" ] || die "缺少 ${ENV_FILE}；请先运行 scripts/init-env.sh"
  set -a
  # .env 由 init-env.sh 生成，保持 shell 兼容格式。
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

postgres_user() {
  docker inspect sub2api-postgres --format '{{range .Config.Env}}{{println .}}{{end}}' \
    | awk -F= '$1 == "POSTGRES_USER" { print substr($0, index($0, "=") + 1) }'
}

compose_export() {
  docker compose \
    --project-name sub2api-ha-export \
    --env-file "${ENV_FILE}" \
    -f "${EXPORT_ROOT}/compose.yaml" \
    "$@"
}

load_recovery_env() {
  load_export_env
  PRIMARY_PROJECT_NAME="${PRIMARY_PROJECT_NAME:-deploy}"
  PRIMARY_COMPOSE_FILE="${PRIMARY_COMPOSE_FILE:-/root/sub2api/deploy/docker-compose.yml}"
  PRIMARY_ENV_FILE="${PRIMARY_ENV_FILE:-/root/sub2api/deploy/.env}"

  [ -f "${PRIMARY_COMPOSE_FILE}" ] || die "缺少 A 原 Compose文件：${PRIMARY_COMPOSE_FILE}"
  [ -f "${PRIMARY_ENV_FILE}" ] || die "缺少 A 原环境文件：${PRIMARY_ENV_FILE}"
  [ -f "${SECRETS_FILE}" ] || die "缺少复制密钥文件：${SECRETS_FILE}"

  set -a
  # A 原 .env 和复制密钥均由管理员维护，必须保持 KEY=VALUE 的 shell 兼容格式。
  # shellcheck disable=SC1090
  . "${PRIMARY_ENV_FILE}"
  # shellcheck disable=SC1090
  . "${SECRETS_FILE}"
  set +a

  [[ "${B_SSH_TARGET:-}" =~ ^[A-Za-z0-9._-]+@[A-Za-z0-9._:-]+$ ]] \
    || die "B_SSH_TARGET 格式无效"
  [[ "${B_DR_ROOT:-}" =~ ^/[A-Za-z0-9._/-]+$ ]] || die "B_DR_ROOT 格式无效"
  if [ -n "${B_SSH_IDENTITY_FILE:-}" ]; then
    [ -f "${B_SSH_IDENTITY_FILE}" ] || die "B SSH私钥不存在：${B_SSH_IDENTITY_FILE}"
  fi
}

compose_primary() {
  docker compose \
    --project-name "${PRIMARY_PROJECT_NAME}" \
    --env-file "${PRIMARY_ENV_FILE}" \
    -f "${PRIMARY_COMPOSE_FILE}" \
    "$@"
}

compose_recovery() {
  docker compose \
    --project-name "${PRIMARY_PROJECT_NAME}" \
    --env-file "${PRIMARY_ENV_FILE}" \
    --env-file "${ENV_FILE}" \
    -f "${PRIMARY_COMPOSE_FILE}" \
    -f "${EXPORT_ROOT}/compose.recovery.yaml" \
    "$@"
}

compose_recovery_promoted() {
  docker compose \
    --project-name "${PRIMARY_PROJECT_NAME}" \
    --env-file "${PRIMARY_ENV_FILE}" \
    --env-file "${ENV_FILE}" \
    -f "${PRIMARY_COMPOSE_FILE}" \
    -f "${EXPORT_ROOT}/compose.recovery.yaml" \
    -f "${EXPORT_ROOT}/compose.recovery-promoted.yaml" \
    "$@"
}

recovery_volume_create() {
  local physical_name="$1"
  local logical_name="$2"
  local compose_version project_label volume_label

  compose_version="$(docker compose version --short)"
  docker volume create \
    --label "com.docker.compose.project=${PRIMARY_PROJECT_NAME}" \
    --label "com.docker.compose.version=${compose_version}" \
    --label "com.docker.compose.volume=${logical_name}" \
    "${physical_name}" >/dev/null

  project_label="$(docker volume inspect --format '{{index .Labels "com.docker.compose.project"}}' "${physical_name}")"
  volume_label="$(docker volume inspect --format '{{index .Labels "com.docker.compose.volume"}}' "${physical_name}")"
  [ "${project_label}" = "${PRIMARY_PROJECT_NAME}" ] || die "卷 ${physical_name} 的 Compose项目标签不匹配"
  [ "${volume_label}" = "${logical_name}" ] || die "卷 ${physical_name} 的 Compose逻辑名称不匹配"
}

container_state() {
  if docker container inspect "$1" >/dev/null 2>&1; then
    docker inspect --format '{{.State.Status}}' "$1"
  else
    printf 'absent\n'
  fi
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

write_state() {
  local name="$1"
  date -Is > "${STATE_DIR}/${name}"
}

ssh_b() {
  if [ -n "${B_SSH_IDENTITY_FILE:-}" ]; then
    ssh \
      -i "${B_SSH_IDENTITY_FILE}" \
      -o IdentitiesOnly=yes \
      -o "ConnectTimeout=${SSH_CONNECT_TIMEOUT:-10}" \
      -o ServerAliveInterval=15 \
      -o ServerAliveCountMax=3 \
      "${B_SSH_TARGET}" \
      "$@"
  else
    ssh \
      -o "ConnectTimeout=${SSH_CONNECT_TIMEOUT:-10}" \
      -o ServerAliveInterval=15 \
      -o ServerAliveCountMax=3 \
      "${B_SSH_TARGET}" \
      "$@"
  fi
}

ssh_b_tty() {
  if [ -n "${B_SSH_IDENTITY_FILE:-}" ]; then
    ssh -tt \
      -i "${B_SSH_IDENTITY_FILE}" \
      -o IdentitiesOnly=yes \
      -o "ConnectTimeout=${SSH_CONNECT_TIMEOUT:-10}" \
      -o ServerAliveInterval=15 \
      -o ServerAliveCountMax=3 \
      "${B_SSH_TARGET}" \
      "$@"
  else
    ssh -tt \
      -o "ConnectTimeout=${SSH_CONNECT_TIMEOUT:-10}" \
      -o ServerAliveInterval=15 \
      -o ServerAliveCountMax=3 \
      "${B_SSH_TARGET}" \
      "$@"
  fi
}
