#!/usr/bin/env bash

set -Eeuo pipefail

usage() {
  cat <<'EOF'
用法：
  apply-app-gate.sh --node A|B [--compose-file <文件>] [--env-file <文件>] [--dry-run]

说明：
  持久设置指定 HA 应用服务的 restart policy 为 "no"，并在线收敛当前容器。
  不会停止、重建或重新创建容器，也不会修改 PostgreSQL、Redis 或 B 原单机部署。

默认目标：
  A: /root/sub2api/deploy/docker-compose.yml, service=sub2api, container=sub2api
  B: /root/sub2api-dr/compose.yaml, service=app-dr, container=sub2api-dr-app
EOF
}

die() {
  printf '错误：%s\n' "$*" >&2
  exit 1
}

node=""
compose_file=""
env_file=""
dry_run=false

while [ "$#" -gt 0 ]; do
  case "$1" in
    --node) node="${2:-}"; shift 2 ;;
    --compose-file) compose_file="${2:-}"; shift 2 ;;
    --env-file) env_file="${2:-}"; shift 2 ;;
    --dry-run) dry_run=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知参数：$1" ;;
  esac
done

case "${node}" in
  A)
    compose_file="${compose_file:-/root/sub2api/deploy/docker-compose.yml}"
    env_file="${env_file:-/root/sub2api/deploy/.env}"
    project_name=deploy
    service_name=sub2api
    container_name=sub2api
    ;;
  B)
    compose_file="${compose_file:-/root/sub2api-dr/compose.yaml}"
    env_file="${env_file:-/root/sub2api-dr/.env}"
    project_name=sub2api-dr
    service_name=app-dr
    container_name=sub2api-dr-app
    ;;
  *) die "--node 必须是 A 或 B" ;;
esac

[ -f "${compose_file}" ] || die "Compose 文件不存在：${compose_file}"
[ -f "${env_file}" ] || die "环境文件不存在：${env_file}"
command -v docker >/dev/null 2>&1 || die "缺少 docker"
command -v yq >/dev/null 2>&1 || die "缺少 yq v4"
yq --version | grep -q 'mikefarah' || die "需要 mikefarah/yq v4"

service_exists="$(SERVICE_NAME="${service_name}" yq -r '.services[strenv(SERVICE_NAME)] != null' "${compose_file}")"
[ "${service_exists}" = "true" ] || die "Compose 缺少 services.${service_name}"
current_restart="$(SERVICE_NAME="${service_name}" yq -r '.services[strenv(SERVICE_NAME)].restart // ""' "${compose_file}")"
container_exists=false
container_id=absent
started_at=absent
runtime_restart=absent
if docker container inspect "${container_name}" >/dev/null 2>&1; then
  container_exists=true
  container_id="$(docker inspect --format '{{.Id}}' "${container_name}")"
  started_at="$(docker inspect --format '{{.State.StartedAt}}' "${container_name}")"
  runtime_restart="$(docker inspect --format '{{.HostConfig.RestartPolicy.Name}}' "${container_name}")"
elif [ "${node}" = "A" ]; then
  die "A 应用容器不存在：${container_name}"
fi

if [ "${dry_run}" = "true" ]; then
  printf 'dry-run：node=%s service=%s container=%s Compose restart=%s，运行态 restart=%s。\n' \
    "${node}" "${service_name}" "${container_name}" "${current_restart:-<未设置>}" "${runtime_restart:-<未设置>}"
  printf 'dry-run：将持久设置 restart="no"，并在线执行 docker update --restart=no；不会修改任何资源。\n'
  printf 'dry-run：容器 ID=%s，启动时间=%s。\n' "${container_id}" "${started_at}"
  exit 0
fi

backup="${compose_file}.pre-ha-gate.$(date -u +%Y%m%dT%H%M%SZ)"
cp -a -- "${compose_file}" "${backup}"
SERVICE_NAME="${service_name}" yq -i '.services[strenv(SERVICE_NAME)].restart = "no"' "${compose_file}"
if ! docker compose --project-name "${project_name}" --env-file "${env_file}" -f "${compose_file}" config >/dev/null; then
  cp -a -- "${backup}" "${compose_file}"
  die "修改后的 Compose 无法渲染，已恢复原文件"
fi
if [ "${container_exists}" = "true" ]; then
  if ! docker update --restart=no "${container_name}" >/dev/null; then
    cp -a -- "${backup}" "${compose_file}"
    die "在线收敛 restart policy 失败，已恢复原 Compose"
  fi
fi

new_compose_restart="$(SERVICE_NAME="${service_name}" yq -r '.services[strenv(SERVICE_NAME)].restart' "${compose_file}")"

[ "${new_compose_restart}" = "no" ] || die "Compose restart policy 未收敛为 no"
if [ "${container_exists}" = "true" ]; then
  new_container_id="$(docker inspect --format '{{.Id}}' "${container_name}")"
  new_started_at="$(docker inspect --format '{{.State.StartedAt}}' "${container_name}")"
  new_runtime_restart="$(docker inspect --format '{{.HostConfig.RestartPolicy.Name}}' "${container_name}")"
  [ "${new_runtime_restart}" = "no" ] || die "运行态 restart policy 未收敛为 no"
  [ "${new_container_id}" = "${container_id}" ] || die "容器 ID 发生变化，停止后续操作并人工检查"
  [ "${new_started_at}" = "${started_at}" ] || die "容器启动时间发生变化，停止后续操作并人工检查"
else
  new_container_id=absent
  new_started_at=absent
fi

printf '节点 %s 应用启动门禁已持久化，容器未重启。\n' "${node}"
printf 'Compose 备份：%s\n' "${backup}"
printf 'container_id=%s started_at=%s restart=no\n' "${new_container_id}" "${new_started_at}"
