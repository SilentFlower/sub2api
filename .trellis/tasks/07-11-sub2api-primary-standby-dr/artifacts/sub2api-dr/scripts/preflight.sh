#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
DR_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
STATE_DIR="${DR_ROOT}/state"
BASELINE_FILE="${STATE_DIR}/baseline-existing-sub2api.txt"
EXISTING_CONTAINERS=(sub2api sub2api-postgres sub2api-redis)
DR_CONTAINERS=(sub2api-dr-app sub2api-dr-postgres sub2api-dr-redis)
DR_VOLUMES=(sub2api-dr-app-data sub2api-dr-postgres-data sub2api-dr-redis-data)

die() {
  printf '错误：%s\n' "$*" >&2
  exit 1
}

capture_snapshot() {
  local target="$1"
  : > "${target}"

  for container in "${EXISTING_CONTAINERS[@]}"; do
    docker inspect --format \
      '{{.Name}}|{{.Id}}|{{.State.StartedAt}}|{{json .Mounts}}|{{json .NetworkSettings.Ports}}|{{json .NetworkSettings.Networks}}' \
      "${container}" >> "${target}"
  done
}

check_existing_service() {
  for container in "${EXISTING_CONTAINERS[@]}"; do
    docker inspect "${container}" >/dev/null 2>&1 || die "现有容器不存在：${container}"
    [ "$(docker inspect --format '{{.State.Running}}' "${container}")" = "true" ] || die "现有容器未运行：${container}"
  done

  curl -fsS http://127.0.0.1:8080/health >/dev/null || die "现有 8080 健康检查失败"

  if ss -H -lnt | awk '$4 ~ /:18080$/ { found = 1 } END { exit(found ? 0 : 1) }'; then
    die "宿主机 18080 已被占用"
  fi
}

check_initial_conflicts() {
  for container in "${DR_CONTAINERS[@]}"; do
    if docker container inspect "${container}" >/dev/null 2>&1; then
      die "发现同名容灾容器：${container}"
    fi
  done

  for volume in "${DR_VOLUMES[@]}"; do
    if docker volume inspect "${volume}" >/dev/null 2>&1; then
      die "发现同名容灾卷：${volume}"
    fi
  done

  if docker network inspect sub2api-dr-network >/dev/null 2>&1; then
    die "发现同名容灾网络：sub2api-dr-network"
  fi
}

verify_no_dr_processes() {
  local running
  running="$(docker ps --format '{{.Names}}' | awk '/^sub2api-dr-/ { print }')"
  [ -z "${running}" ] || die "发现运行中的容灾容器：${running}"
}

verify_standby_processes() {
  local container
  local unexpected

  for container in sub2api-dr-postgres sub2api-dr-redis; do
    docker container inspect "${container}" >/dev/null 2>&1 || die "容灾数据库容器不存在：${container}"
    [ "$(docker inspect --format '{{.State.Running}}' "${container}")" = "true" ] || die "容灾数据库容器未运行：${container}"
  done

  if docker container inspect sub2api-dr-app >/dev/null 2>&1 \
    && [ "$(docker inspect --format '{{.State.Running}}' sub2api-dr-app)" = "true" ]; then
    die "备用应用不应在备库阶段运行"
  fi

  unexpected="$(docker ps --format '{{.Names}}' | awk '
    /^sub2api-dr-/ && $0 != "sub2api-dr-postgres" && $0 != "sub2api-dr-redis" { print }
  ')"
  [ -z "${unexpected}" ] || die "发现非预期运行中的容灾容器：${unexpected}"
}

verify_baseline_unchanged() {
  local current

  current="$(mktemp)"
  capture_snapshot "${current}"
  if ! diff -u "${BASELINE_FILE}" "${current}"; then
    rm -f "${current}"
    die "B 现有 Sub2API容器基线发生变化"
  fi
  rm -f "${current}"
}

main() {
  local mode="${1:-check}"

  command -v docker >/dev/null 2>&1 || die "缺少 docker"
  command -v curl >/dev/null 2>&1 || die "缺少 curl"
  command -v ss >/dev/null 2>&1 || die "缺少 ss"
  [ -f /root/sub2api/docker-compose.yml ] || die "B 现有 /root/sub2api/docker-compose.yml 不存在"
  mkdir -p "${STATE_DIR}"
  check_existing_service

  case "${mode}" in
    capture)
      [ ! -e "${BASELINE_FILE}" ] || die "基线已存在，拒绝覆盖：${BASELINE_FILE}"
      check_initial_conflicts
      capture_snapshot "${BASELINE_FILE}"
      verify_no_dr_processes
      printf '基线已记录：%s\n' "${BASELINE_FILE}"
      ;;
    verify)
      [ -f "${BASELINE_FILE}" ] || die "缺少基线：${BASELINE_FILE}"
      verify_baseline_unchanged
      verify_no_dr_processes
      printf '基线比对通过，现有容器未变化，8080 正常，18080 空闲。\n'
      ;;
    verify-standby)
      [ -f "${BASELINE_FILE}" ] || die "缺少基线：${BASELINE_FILE}"
      verify_baseline_unchanged
      verify_standby_processes
      printf '备库基线比对通过，现有容器未变化，数据库备库运行，备用应用停止，18080 空闲。\n'
      ;;
    check)
      verify_no_dr_processes
      printf '预检通过，8080 正常，18080 空闲。\n'
      ;;
    *)
      die "用法：$0 capture|verify|verify-standby|check"
      ;;
  esac
}

main "$@"
