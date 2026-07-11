#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

volume_is_empty() {
  docker run --rm \
    --entrypoint /bin/sh \
    -v sub2api-dr-app-data:/target \
    "${SUB2API_IMAGE}" \
    -ec '[ -z "$(find /target -mindepth 1 -maxdepth 1 -print -quit)" ]'
}

validate_archive() {
  local archive="$1"
  local listing

  listing="$(tar -tzf "${archive}")" || die "应用数据归档无法读取"
  [ -n "${listing}" ] || die "应用数据归档为空"

  if printf '%s\n' "${listing}" | grep -Eq '(^/|(^|/)\.\.(/|$)|^(\./)?logs(/|$))'; then
    die "应用数据归档包含绝对路径、上级路径或 logs 目录"
  fi
  printf '%s\n' "${listing}" | grep -Eq '^(\./)?\.installed$' || die "应用数据归档缺少 .installed"
  printf '%s\n' "${listing}" | grep -Eq '^(\./)?config\.yaml$' || die "应用数据归档缺少 config.yaml"
}

main() {
  local archive="${1:-}"
  local archive_path

  require_command docker
  require_command grep
  require_command readlink
  require_command tar
  load_runtime_env
  require_var SUB2API_IMAGE

  [ -f "${archive}" ] || die "用法：$0 <A应用数据归档.tar.gz>"
  archive_path="$(readlink -f "${archive}")"
  validate_archive "${archive_path}"

  if docker container inspect sub2api-dr-app >/dev/null 2>&1; then
    die "sub2api-dr-app 已存在；导入阶段要求备用应用容器不存在"
  fi

  compose_volume_create sub2api-dr-app-data app-data
  volume_is_empty || die "sub2api-dr-app-data 非空，拒绝覆盖"

  docker run --rm \
    --entrypoint /bin/sh \
    -v sub2api-dr-app-data:/target \
    -v "${archive_path}:/import/app-data.tar.gz:ro" \
    "${SUB2API_IMAGE}" \
    -ec '
      tar -xzf /import/app-data.tar.gz -C /target
      [ -f /target/.installed ]
      [ -f /target/config.yaml ]
      [ ! -e /target/logs ]
    '

  write_state app-data-imported
  printf 'A 的非日志应用数据已导入 sub2api-dr-app-data。\n'
}

main "$@"
