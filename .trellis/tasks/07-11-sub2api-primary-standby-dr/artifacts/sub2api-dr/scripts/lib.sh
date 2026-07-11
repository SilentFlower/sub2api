#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
DR_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
STATE_DIR="${DR_ROOT}/state"
ENV_FILE="${DR_ROOT}/.env"
RELEASE_STATE_FILE="${STATE_DIR}/release-image.env"

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

verify_release_image_ready() {
  local configured_digest release_digest expected_id actual_id

  configured_digest="$(env_value "${ENV_FILE}" SUB2API_IMAGE)" \
    || die "B 环境文件缺少 SUB2API_IMAGE"
  validate_image_digest "${configured_digest}" \
    || die "B 容灾应用镜像不是固定 digest：${configured_digest}"
  image_is_cached "${configured_digest}" \
    || die "B 尚未缓存容灾应用镜像：${configured_digest}"
  release_digest="$(release_state_value APP_IMAGE_DIGEST 2>/dev/null || true)"
  [ "${release_digest}" = "${configured_digest}" ] \
    || die "B 容灾应用镜像与最近发布同步记录不一致"

  if docker container inspect sub2api-dr-app >/dev/null 2>&1 \
    && [ "$(docker inspect --format '{{.State.Running}}' sub2api-dr-app)" = "true" ]; then
    expected_id="$(docker image inspect --format '{{.Id}}' "${configured_digest}")"
    actual_id="$(docker inspect --format '{{.Image}}' sub2api-dr-app)"
    [ "${actual_id}" = "${expected_id}" ] \
      || die "B 运行中的容灾应用镜像与发布配置不一致"
  fi
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
