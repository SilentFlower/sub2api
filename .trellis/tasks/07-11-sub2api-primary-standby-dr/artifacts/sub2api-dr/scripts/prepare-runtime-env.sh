#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

require_source_value() {
  local file="$1"
  local name="$2"
  local value

  value="$(env_value "${file}" "${name}")" || die "A 环境文件缺少 ${name}"
  case "${value}" in
    ""|'""'|"''") die "A 环境文件中的 ${name} 不能为空" ;;
  esac
}

require_source_key() {
  local file="$1"
  local name="$2"

  env_value "${file}" "${name}" >/dev/null || die "A 环境文件缺少 ${name}"
}

require_digest_image() {
  local name="$1"
  local value="${!name:-}"

  [ -n "${value}" ] || die "环境变量 ${name} 不能为空"
  case "${value}" in
    *@sha256:*) ;;
    *) die "环境变量 ${name} 必须使用仓库@sha256固定摘要" ;;
  esac
}

main() {
  local source_env="${1:-}"
  local replication_secrets="${2:-}"
  local redis_password
  local replication_line
  local temp_env

  require_command awk
  require_command chmod
  require_command grep
  require_command mktemp
  require_command mv

  [ -f "${source_env}" ] || die "用法：$0 <A应用.env> <A复制secrets.env>"
  [ -f "${replication_secrets}" ] || die "复制密钥文件不存在：${replication_secrets}"
  require_var A_REPLICATION_HOST
  require_digest_image SUB2API_IMAGE
  require_digest_image POSTGRES_IMAGE
  require_digest_image REDIS_IMAGE

  require_source_value "${source_env}" POSTGRES_USER
  require_source_value "${source_env}" POSTGRES_PASSWORD
  require_source_value "${source_env}" POSTGRES_DB
  # 当前版本可从 config.yaml 和数据库恢复持久化 JWT密钥，空环境值必须按 A 原样保留。
  require_source_key "${source_env}" JWT_SECRET

  redis_password="$(env_value "${source_env}" REDIS_PASSWORD)" || die "A 环境文件缺少 REDIS_PASSWORD"
  case "${redis_password}" in
    ""|'""'|"''") ;;
    *) die "A 当前 Redis存在密码，需先扩展脚本的 masterauth处理后再继续" ;;
  esac

  replication_line="$(awk '/^POSTGRES_REPLICATION_PASSWORD=/{line=$0} END{print line}' "${replication_secrets}")"
  [ -n "${replication_line}" ] || die "复制密钥文件缺少 POSTGRES_REPLICATION_PASSWORD"
  case "${replication_line#*=}" in
    ""|'""'|"''") die "POSTGRES_REPLICATION_PASSWORD 不能为空" ;;
  esac

  umask 077
  temp_env="$(mktemp "${DR_ROOT}/.env.tmp.XXXXXX")"
  trap 'rm -f "${temp_env}"' EXIT

  {
    # B 容灾覆盖键必须只出现一次，避免 shell 与 Compose 对重复键产生隐式最后值语义。
    awk '
      /^(COMPOSE_PROJECT_NAME|DR_BIND_HOST|DR_APP_PORT|SUB2API_IMAGE|POSTGRES_IMAGE|REDIS_IMAGE|A_REPLICATION_HOST|A_POSTGRES_REPLICATION_PORT|POSTGRES_REPLICATION_USER|POSTGRES_REPLICATION_PASSWORD|POSTGRES_REPLICATION_SLOT|REDIS_MASTER_PASSWORD|A_REDIS_REPLICATION_PORT)=/ { next }
      { print }
    ' "${source_env}"
    printf '\n# B 容灾栈运行时覆盖\n'
    printf 'COMPOSE_PROJECT_NAME=sub2api-dr\n'
    printf 'DR_BIND_HOST=0.0.0.0\n'
    printf 'DR_APP_PORT=18080\n'
    printf 'SUB2API_IMAGE=%s\n' "${SUB2API_IMAGE}"
    printf 'POSTGRES_IMAGE=%s\n' "${POSTGRES_IMAGE}"
    printf 'REDIS_IMAGE=%s\n' "${REDIS_IMAGE}"
    printf 'A_REPLICATION_HOST=%s\n' "${A_REPLICATION_HOST}"
    printf 'A_POSTGRES_REPLICATION_PORT=%s\n' "${A_POSTGRES_REPLICATION_PORT:-15432}"
    printf 'POSTGRES_REPLICATION_USER=%s\n' "${POSTGRES_REPLICATION_USER:-sub2api_replica}"
    printf '%s\n' "${replication_line}"
    printf 'POSTGRES_REPLICATION_SLOT=%s\n' "${POSTGRES_REPLICATION_SLOT:-sub2api_b_standby}"
    printf 'REDIS_MASTER_PASSWORD=\n'
    printf 'A_REDIS_REPLICATION_PORT=%s\n' "${A_REDIS_REPLICATION_PORT:-16379}"
  } > "${temp_env}"

  if grep -q 'REPLACE_WITH_' "${temp_env}"; then
    die "生成的 .env 仍包含占位值"
  fi

  chmod 0600 "${temp_env}"
  mv -f "${temp_env}" "${ENV_FILE}"
  trap - EXIT
  printf 'B 容灾栈真实 .env 已生成，权限为 0600。\n'
}

main "$@"
