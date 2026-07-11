#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
EXPORT_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
STATE_DIR="${EXPORT_ROOT}/state"
ENV_FILE="${EXPORT_ROOT}/.env"
RELEASE_STATE_FILE="${STATE_DIR}/release-image.env"
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

env_value() {
  local file="$1"
  local name="$2"

  awk -v name="${name}" '
    index($0, name "=") == 1 {
      value = substr($0, length(name) + 2)
      found = 1
    }
    END {
      if (!found) {
        exit 1
      }
      print value
    }
  ' "${file}"
}

validate_image_reference() {
  [[ "$1" =~ ^[A-Za-z0-9][A-Za-z0-9._/@:+-]*$ ]]
}

validate_image_digest() {
  [[ "$1" =~ ^[A-Za-z0-9][A-Za-z0-9._/:+-]*@sha256:[a-f0-9]{64}$ ]]
}

image_repository_from_ref() {
  local ref="$1"
  local repository last_component

  validate_image_reference "${ref}" || return 1
  repository="${ref%%@*}"
  last_component="${repository##*/}"
  if [[ "${last_component}" == *:* ]]; then
    repository="${repository%:*}"
  fi
  [ -n "${repository}" ] || return 1
  printf '%s\n' "${repository}"
}

resolve_running_image_digest() {
  local container="$1"
  local source_ref image_id repository candidate candidate_id match=""

  docker container inspect "${container}" >/dev/null 2>&1 || {
    printf '无法读取运行容器：%s\n' "${container}" >&2
    return 1
  }
  source_ref="$(docker inspect --format '{{.Config.Image}}' "${container}")"
  image_id="$(docker inspect --format '{{.Image}}' "${container}")"
  repository="$(image_repository_from_ref "${source_ref}")" || {
    printf '容器 %s 的镜像引用格式无效：%s\n' "${container}" "${source_ref}" >&2
    return 1
  }

  while IFS= read -r candidate; do
    [ -n "${candidate}" ] || continue
    validate_image_digest "${candidate}" || continue
    [[ "${candidate}" == "${repository}@sha256:"* ]] || continue
    candidate_id="$(docker image inspect --format '{{.Id}}' "${candidate}" 2>/dev/null || true)"
    [ "${candidate_id}" = "${image_id}" ] || continue
    if [ -n "${match}" ] && [ "${candidate}" != "${match}" ]; then
      printf '运行镜像匹配到多个同仓库 digest：%s、%s\n' "${match}" "${candidate}" >&2
      return 1
    fi
    match="${candidate}"
  done < <(docker image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' "${image_id}" 2>/dev/null || true)

  [ -n "${match}" ] || {
    printf '运行镜像没有与仓库 %s 匹配的可拉取 RepoDigest\n' "${repository}" >&2
    return 1
  }
  printf '%s\n' "${match}"
}

image_is_cached() {
  validate_image_digest "$1" || return 1
  docker image inspect "$1" >/dev/null 2>&1
}

release_state_value() {
  local name="$1"

  [ -f "${RELEASE_STATE_FILE}" ] || return 1
  env_value "${RELEASE_STATE_FILE}" "${name}"
}

replace_env_value() {
  local file="$1"
  local name="$2"
  local value="$3"
  local temp_file

  [[ "${name}" =~ ^[A-Z][A-Z0-9_]*$ ]] || die "环境变量名称格式无效：${name}"
  [ -f "${file}" ] || die "环境文件不存在：${file}"
  temp_file="$(mktemp "${file}.tmp.XXXXXX")"
  if ! awk -v name="${name}" -v value="${value}" '
    BEGIN { found = 0 }
    index($0, name "=") == 1 {
      if (!found) {
        print name "=" value
      }
      found = 1
      next
    }
    { print }
    END { if (!found) exit 2 }
  ' "${file}" > "${temp_file}"; then
    rm -f "${temp_file}"
    die "环境文件缺少 ${name}，拒绝追加未知配置"
  fi
  chmod 0600 "${temp_file}"
  mv -f "${temp_file}" "${file}"
}

write_release_state() {
  local digest="$1"
  local previous_digest="$2"
  local source_ref="$3"
  local synced_at="${4:-$(date -Is)}"
  local temp_file

  validate_image_digest "${digest}" || die "发布镜像 digest格式无效：${digest}"
  [ -z "${previous_digest}" ] || validate_image_digest "${previous_digest}" \
    || die "上一发布镜像 digest格式无效：${previous_digest}"
  validate_image_reference "${source_ref}" || die "来源镜像引用格式无效：${source_ref}"
  mkdir -p "${STATE_DIR}"
  temp_file="$(mktemp "${RELEASE_STATE_FILE}.tmp.XXXXXX")"
  {
    printf 'APP_IMAGE_DIGEST=%s\n' "${digest}"
    printf 'PREVIOUS_APP_IMAGE_DIGEST=%s\n' "${previous_digest}"
    printf 'SOURCE_IMAGE_REF=%s\n' "${source_ref}"
    printf 'SYNCED_AT=%s\n' "${synced_at}"
  } > "${temp_file}"
  chmod 0600 "${temp_file}"
  mv -f "${temp_file}" "${RELEASE_STATE_FILE}"
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
