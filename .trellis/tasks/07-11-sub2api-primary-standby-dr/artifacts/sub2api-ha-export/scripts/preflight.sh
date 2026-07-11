#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

BASELINE_FILE="${STATE_DIR}/baseline-existing-sub2api.txt"
EXISTING_CONTAINERS=(sub2api sub2api-postgres sub2api-redis)
EXPORT_CONTAINERS=(sub2api-ha-postgres-export sub2api-ha-redis-export)

port_is_listening() {
  local port="$1"
  ss -H -lnt | awk -v suffix=":${port}" '$4 ~ suffix "$" { found = 1 } END { exit(found ? 0 : 1) }'
}

capture_snapshot() {
  local target="$1"
  local pg_user
  pg_user="$(postgres_user)"
  [ -n "${pg_user}" ] || die "无法识别 PostgreSQL管理员用户"

  {
    for container in "${EXISTING_CONTAINERS[@]}"; do
      docker inspect --format \
        'container|{{.Name}}|{{.Id}}|{{.State.StartedAt}}|{{json .NetworkSettings.Ports}}|{{json .NetworkSettings.Networks}}' \
        "${container}"
      docker inspect --format \
        '{{range .Mounts}}{{printf "%s|%s|%s|%t\n" .Destination .Name .Source .RW}}{{end}}' \
        "${container}" \
        | sort \
        | sed "s~^~mount|${container}|~"
    done
    printf 'compose_sha256=%s\n' "$(sha256sum /root/sub2api/deploy/docker-compose.yml | awk '{print $1}')"
    printf 'env_sha256=%s\n' "$(sha256sum /root/sub2api/deploy/.env | awk '{print $1}')"
    printf 'postgres_started_at=%s\n' "$(docker exec sub2api-postgres psql -U "${pg_user}" -d postgres -Atc 'SELECT pg_postmaster_start_time();')"
  } > "${target}"
}

verify_legacy_identity() {
  local legacy_file="$1"
  local pg_user expected actual index container

  pg_user="$(postgres_user)"
  for index in 1 2 3; do
    container="${EXISTING_CONTAINERS[$((index - 1))]}"
    expected="$(sed -n "${index}p" "${legacy_file}" | cut -d'|' -f1-3)"
    actual="$(docker inspect --format '{{.Name}}|{{.Id}}|{{.State.StartedAt}}' "${container}")"
    [ "${expected}" = "${actual}" ] || die "旧基线中的容器身份或启动时间不匹配：${container}"
  done

  expected="$(sed -n 's/^compose_sha256=//p' "${legacy_file}")"
  actual="$(sha256sum /root/sub2api/deploy/docker-compose.yml | awk '{print $1}')"
  [ "${expected}" = "${actual}" ] || die "A 原 Compose 文件哈希发生变化"
  expected="$(sed -n 's/^env_sha256=//p' "${legacy_file}")"
  actual="$(sha256sum /root/sub2api/deploy/.env | awk '{print $1}')"
  [ "${expected}" = "${actual}" ] || die "A 原环境文件哈希发生变化"
  expected="$(sed -n 's/^postgres_started_at=//p' "${legacy_file}")"
  actual="$(docker exec sub2api-postgres psql -U "${pg_user}" -d postgres -Atc 'SELECT pg_postmaster_start_time();')"
  [ "${expected}" = "${actual}" ] || die "PostgreSQL进程启动时间发生变化"
}

check_existing_service() {
  for container in "${EXISTING_CONTAINERS[@]}"; do
    docker inspect "${container}" >/dev/null 2>&1 || die "现有容器不存在：${container}"
    [ "$(docker inspect --format '{{.State.Running}}' "${container}")" = "true" ] || die "现有容器未运行：${container}"
  done
  curl -fsS http://127.0.0.1:8080/health >/dev/null || die "A 原应用 8080 健康检查失败"
}

main() {
  local mode="${1:-check}"
  local port current legacy_file new_baseline

  require_command docker
  require_command curl
  require_command ss
  require_command sha256sum
  load_export_env
  check_existing_service

  case "${mode}" in
    capture)
      [ ! -e "${BASELINE_FILE}" ] || die "基线已存在，拒绝覆盖：${BASELINE_FILE}"
      for container in "${EXPORT_CONTAINERS[@]}"; do
        docker container inspect "${container}" >/dev/null 2>&1 && die "发现同名转发容器：${container}"
      done
      for port in "${POSTGRES_EXPORT_PORT}" "${REDIS_EXPORT_PORT}"; do
        port_is_listening "${port}" && die "端口 ${port} 已被占用"
      done
      capture_snapshot "${BASELINE_FILE}"
      printf 'A 原部署基线已记录。\n'
      ;;
    verify)
      [ -f "${BASELINE_FILE}" ] || die "缺少基线：${BASELINE_FILE}"
      current="$(mktemp)"
      capture_snapshot "${current}"
      if ! diff -u "${BASELINE_FILE}" "${current}"; then
        rm -f "${current}"
        die "A 原 Sub2API容器、配置文件或 PostgreSQL进程基线发生变化"
      fi
      rm -f "${current}"
      for container in "${EXPORT_CONTAINERS[@]}"; do
        [ "$(docker inspect --format '{{.State.Running}}' "${container}" 2>/dev/null || true)" = "true" ] || die "转发容器未运行：${container}"
      done
      for port in "${POSTGRES_EXPORT_PORT}" "${REDIS_EXPORT_PORT}"; do
        port_is_listening "${port}" || die "转发端口未监听：${port}"
      done
      printf 'A 基线比对通过，原容器与 PostgreSQL进程均未重启。\n'
      ;;
    migrate)
      [ -f "${BASELINE_FILE}" ] || die "缺少待迁移的旧基线：${BASELINE_FILE}"
      legacy_file="${STATE_DIR}/baseline-existing-sub2api.raw-before.txt"
      [ ! -e "${legacy_file}" ] || die "原始旧基线已存在，拒绝重复迁移"
      verify_legacy_identity "${BASELINE_FILE}"
      new_baseline="$(mktemp)"
      capture_snapshot "${new_baseline}"
      cp "${BASELINE_FILE}" "${legacy_file}"
      mv "${new_baseline}" "${BASELINE_FILE}"
      printf '旧基线身份验证通过，已迁移为稳定排序格式并保留原始副本。\n'
      ;;
    check)
      printf 'A 原服务预检通过。\n'
      ;;
    *)
      die "用法：$0 capture|verify|migrate|check"
      ;;
  esac
}

main "$@"
