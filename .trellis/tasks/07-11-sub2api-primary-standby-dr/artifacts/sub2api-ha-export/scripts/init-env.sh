#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
EXPORT_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
ENV_FILE="${EXPORT_ROOT}/.env"
EXAMPLE_FILE="${EXPORT_ROOT}/.env.example"

die() {
  printf '错误：%s\n' "$*" >&2
  exit 1
}

validate_cidr() {
  local cidr="$1"
  local ip octet
  local -a octets

  [[ "${cidr}" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}/32$ ]] || return 1
  ip="${cidr%/32}"
  IFS='.' read -r -a octets <<< "${ip}"
  [ "${#octets[@]}" -eq 4 ] || return 1
  for octet in "${octets[@]}"; do
    ((10#${octet} >= 0 && 10#${octet} <= 255)) || return 1
  done
}

main() {
  local source_cidr="${1:-}"
  validate_cidr "${source_cidr}" || die "用法：$0 <B公网IPv4/32>"
  [ -f "${EXAMPLE_FILE}" ] || die "缺少 ${EXAMPLE_FILE}"

  if [ -f "${ENV_FILE}" ]; then
    current="$(awk -F= '$1 == "B_SOURCE_CIDR" { print substr($0, index($0, "=") + 1) }' "${ENV_FILE}")"
    [ "${current}" = "${source_cidr}" ] || die "现有 .env 的 B_SOURCE_CIDR 与本次输入不同，拒绝覆盖"
    chmod 0600 "${ENV_FILE}"
    printf '现有 .env 已通过来源地址校验。\n'
    return 0
  fi

  umask 077
  cp "${EXAMPLE_FILE}" "${ENV_FILE}"
  sed -i "s|REPLACE_WITH_B_PUBLIC_IP/32|${source_cidr}|" "${ENV_FILE}"
  chmod 0600 "${ENV_FILE}"
  printf 'A 复制出口环境文件已生成。\n'
}

main "$@"
