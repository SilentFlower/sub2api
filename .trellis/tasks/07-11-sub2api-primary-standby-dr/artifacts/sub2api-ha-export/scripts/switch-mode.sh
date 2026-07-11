#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

usage() {
  cat <<'EOF'
用法：
  switch-mode.sh status
  switch-mode.sh sync-release [--dry-run]
  switch-mode.sh prepare-from-b [--dry-run]
  switch-mode.sh cutback-to-a [--dry-run]
  switch-mode.sh restore-b-standby [--dry-run]

说明：
  status             只读显示 A 本地角色和 B 容灾角色。
  sync-release       A 更新健康后，把实际运行镜像 digest同步到双端容灾配置。
  prepare-from-b     B 已接管后，使用新命名卷把 A 重建为 B 的备节点。
  cutback-to-a       冻结 B 写入、等待 A 追平，再提升 A 并启动 A 应用。
  restore-b-standby  A 稳定为主后，从 A 全量重建 B 并恢复 A 主 B 备。

每个变更命令都支持 --dry-run。实际操作仍会要求对应阶段的确认口令。
EOF
}

field_value() {
  local data="$1"
  local key="$2"

  printf '%s\n' "${data}" | awk -F= -v key="${key}" '$1 == key { print substr($0, index($0, "=") + 1) }'
}

local_state() {
  "${SCRIPT_DIR}/verify-cutback.sh" --machine
}

b_state() {
  ssh_b "cd '${B_DR_ROOT}' && ./scripts/switch-mode.sh status --machine"
}

image_sync_state() {
  local local_output="$1"
  local remote_output="$2"
  local local_mode b_mode expected_digest app_digest recovery_digest recovery_cached
  local local_release dr_digest dr_cached dr_release running_dr value

  local_mode="$(field_value "${local_output}" mode)"
  b_mode="$(field_value "${remote_output}" mode)"
  app_digest="$(field_value "${local_output}" app_image_digest)"
  recovery_digest="$(field_value "${local_output}" recovery_image_digest)"
  recovery_cached="$(field_value "${local_output}" recovery_image_cached)"
  local_release="$(field_value "${local_output}" release_image_digest)"
  dr_digest="$(field_value "${remote_output}" app_image_digest)"
  dr_cached="$(field_value "${remote_output}" app_image_cached)"
  dr_release="$(field_value "${remote_output}" release_image_digest)"
  running_dr="$(field_value "${remote_output}" running_app_image_digest)"

  case "${local_mode}" in
    legacy-active|active-recovered) expected_digest="${app_digest}" ;;
    standby-from-b|cutback-postgres-promoted|active-recovered-stopped)
      case "${b_mode}" in
        active|active-stopped) expected_digest="${dr_digest}" ;;
        *) printf 'unknown\n'; return 0 ;;
      esac
      ;;
    *) printf 'unknown\n'; return 0 ;;
  esac

  for value in "${expected_digest}" "${recovery_digest}" "${local_release}" "${dr_digest}" "${dr_release}"; do
    case "${value}" in
      ""|unknown|absent) printf 'unknown\n'; return 0 ;;
    esac
  done
  if ! validate_image_digest "${expected_digest}" \
    || ! validate_image_digest "${recovery_digest}" \
    || ! validate_image_digest "${local_release}" \
    || ! validate_image_digest "${dr_digest}" \
    || ! validate_image_digest "${dr_release}"; then
    printf 'drift\n'
    return 0
  fi
  if [ "${expected_digest}" != "${recovery_digest}" ] \
    || [ "${expected_digest}" != "${local_release}" ] \
    || [ "${expected_digest}" != "${dr_digest}" ] \
    || [ "${expected_digest}" != "${dr_release}" ] \
    || [ "${recovery_cached}" != "yes" ] \
    || [ "${dr_cached}" != "yes" ]; then
    printf 'drift\n'
    return 0
  fi
  if [ "${b_mode}" = "active" ] && [ "${running_dr}" != "${dr_digest}" ]; then
    printf 'drift\n'
    return 0
  fi
  printf 'ok\n'
}

validate_b_release_image() {
  local remote_output="$1"
  local b_mode digest cached release_digest running_digest

  b_mode="$(field_value "${remote_output}" mode)"
  digest="$(field_value "${remote_output}" app_image_digest)"
  cached="$(field_value "${remote_output}" app_image_cached)"
  release_digest="$(field_value "${remote_output}" release_image_digest)"
  running_digest="$(field_value "${remote_output}" running_app_image_digest)"
  validate_image_digest "${digest}" || die "B 容灾应用镜像不是固定 digest：${digest:-unknown}"
  [ "${cached}" = "yes" ] || die "B 尚未缓存容灾应用镜像：${digest}"
  [ "${release_digest}" = "${digest}" ] || die "B 容灾应用镜像与发布同步记录不一致"
  if [ "${b_mode}" = "active" ]; then
    [ "${running_digest}" = "${digest}" ] || die "B 运行中的容灾应用镜像与发布配置不一致"
  fi
  printf '%s\n' "${digest}"
}

set_local_release_image() {
  local digest="$1"
  local source_ref="$2"
  local current_digest current_release current_previous current_source previous_digest

  current_digest="$(env_value "${ENV_FILE}" SUB2API_IMAGE)" \
    || die "A 容灾环境文件缺少 SUB2API_IMAGE"
  current_release="$(release_state_value APP_IMAGE_DIGEST 2>/dev/null || true)"
  current_previous="$(release_state_value PREVIOUS_APP_IMAGE_DIGEST 2>/dev/null || true)"
  current_source="$(release_state_value SOURCE_IMAGE_REF 2>/dev/null || true)"
  if [ "${current_digest}" = "${digest}" ] \
    && [ "${current_release}" = "${digest}" ] \
    && [ "${current_source}" = "${source_ref}" ]; then
    return 0
  fi

  previous_digest="${current_previous}"
  if [ "${current_digest}" != "${digest}" ] && validate_image_digest "${current_digest}"; then
    previous_digest="${current_digest}"
  fi
  replace_env_value "${ENV_FILE}" SUB2API_IMAGE "${digest}"
  write_release_state "${digest}" "${previous_digest}" "${source_ref}"
}

show_status() {
  local machine="${1:-false}"
  local local_output remote_output remote_human sync_state

  local_output="$(local_state)"
  if ! remote_output="$(b_state)"; then
    if [ "${machine}" = "true" ]; then
      printf '%s\n' "${local_output}"
      printf 'dr_image_digest=unknown\n'
      printf 'dr_image_cached=unknown\n'
      printf 'dr_release_image_digest=unknown\n'
      printf 'dr_release_synced_at=unknown\n'
      printf 'image_sync=unknown\n'
    else
      "${SCRIPT_DIR}/verify-cutback.sh"
      printf '%s\n' '警告：无法读取 B 状态；A 本地状态如上。' >&2
    fi
    return 2
  fi
  sync_state="$(image_sync_state "${local_output}" "${remote_output}")"

  if [ "${machine}" = "true" ]; then
    printf '%s\n' "${local_output}"
    printf 'dr_image_digest=%s\n' "$(field_value "${remote_output}" app_image_digest)"
    printf 'dr_image_cached=%s\n' "$(field_value "${remote_output}" app_image_cached)"
    printf 'dr_release_image_digest=%s\n' "$(field_value "${remote_output}" release_image_digest)"
    printf 'dr_release_synced_at=%s\n' "$(field_value "${remote_output}" release_synced_at)"
    printf 'image_sync=%s\n' "${sync_state}"
    return 0
  fi

  "${SCRIPT_DIR}/verify-cutback.sh"
  printf '%s\n' '--- B 容灾节点 ---'
  if ! remote_human="$(ssh_b "cd '${B_DR_ROOT}' && ./scripts/switch-mode.sh status")"; then
    printf '%s\n' '警告：无法读取 B 人类可读状态；机器状态已获取。' >&2
    return 2
  fi
  printf '%s\n' "${remote_human}"
  printf '双端发布镜像：A运行=%s A恢复=%s B配置=%s B缓存=%s sync=%s\n' \
    "$(field_value "${local_output}" app_image_digest)" \
    "$(field_value "${local_output}" recovery_image_digest)" \
    "$(field_value "${remote_output}" app_image_digest)" \
    "$(field_value "${remote_output}" app_image_cached)" \
    "${sync_state}"
}

sync_release() {
  local dry_run="$1"
  local local_output remote_output local_mode b_mode source_ref digest sync_state
  local local_image_id remote_image_id remote_cached

  local_output="$(local_state)"
  local_mode="$(field_value "${local_output}" mode)"
  case "${local_mode}" in
    legacy-active|active-recovered) ;;
    *) die "A 当前模式为 ${local_mode}，只允许启用主节点同步发布镜像" ;;
  esac
  curl -fsS "http://127.0.0.1:${SERVER_PORT:-8080}/health" >/dev/null \
    || die "A 应用健康检查失败，拒绝同步发布镜像"

  remote_output="$(b_state)"
  b_mode="$(field_value "${remote_output}" mode)"
  [ "${b_mode}" = "standby" ] || die "B 当前模式为 ${b_mode}，只允许向 standby同步发布镜像"
  [ "$(field_value "${remote_output}" postgres_recovery)" = "t" ] \
    || die "B PostgreSQL不是可用备库"
  [ "$(field_value "${remote_output}" redis_role)" = "slave" ] \
    || die "B Redis不是从库"
  [ "$(field_value "${remote_output}" redis_link)" = "up" ] \
    || die "B Redis主链路未连接"
  [ "$(field_value "${remote_output}" redis_sync)" = "0" ] \
    || die "B Redis仍在执行同步"
  ssh_b "cd '${B_DR_ROOT}' && ./scripts/verify-replication.sh >/dev/null" \
    || die "B 数据库复制健康检查失败，拒绝同步发布镜像"

  source_ref="$(docker inspect --format '{{.Config.Image}}' sub2api)"
  validate_image_reference "${source_ref}" || die "A 运行镜像引用格式无效：${source_ref}"
  digest="$(resolve_running_image_digest sub2api)" || die "无法解析 A 当前运行镜像的可拉取 digest"
  local_image_id="$(docker inspect --format '{{.Image}}' sub2api)"
  remote_cached="$(field_value "${remote_output}" app_image_cached)"
  sync_state="$(image_sync_state "${local_output}" "${remote_output}")"

  printf 'A 当前运行镜像：source=%s digest=%s\n' "${source_ref}" "${digest}"
  printf '当前容灾配置：A=%s B=%s B缓存=%s sync=%s\n' \
    "$(field_value "${local_output}" recovery_image_digest)" \
    "$(field_value "${remote_output}" app_image_digest)" \
    "${remote_cached}" "${sync_state}"
  if [ "${dry_run}" = "true" ]; then
    if [ "${sync_state}" = "ok" ]; then
      printf 'dry-run：双端已经同步，无需更新。\n'
    else
      printf 'dry-run：将让 B 拉取精确 digest，再只更新双端 SUB2API_IMAGE和发布状态。\n'
    fi
    printf 'dry-run 完成：没有拉取镜像、修改环境文件、重启服务或启动 B 应用。\n'
    return 0
  fi
  if [ "${sync_state}" = "ok" ]; then
    printf '双端发布镜像已经同步，无需重复更新。\n'
    return 0
  fi

  ssh_b "docker pull '${digest}' >/dev/null"
  remote_image_id="$(ssh_b "docker image inspect --format '{{.Id}}' '${digest}'")"
  [ "${remote_image_id}" = "${local_image_id}" ] \
    || die "B 拉取镜像后的 image ID与 A 当前运行镜像不一致"
  ssh_b "cd '${B_DR_ROOT}' && ./scripts/set-release-image.sh '${digest}' '${source_ref}'"
  set_local_release_image "${digest}" "${source_ref}"

  local_output="$(local_state)"
  remote_output="$(b_state)"
  sync_state="$(image_sync_state "${local_output}" "${remote_output}")"
  [ "${sync_state}" = "ok" ] || die "双端发布镜像同步后状态仍为 ${sync_state}"
  printf '发布镜像同步完成：%s；A/B 数据库和应用运行状态未改变。\n' "${digest}"
}

ensure_recovery_image_from_b() {
  local remote_output="$1"
  local dry_run="$2"
  local digest source_ref current_digest local_image_id remote_image_id

  digest="$(validate_b_release_image "${remote_output}")"
  source_ref="$(field_value "${remote_output}" release_source_ref)"
  if ! validate_image_reference "${source_ref}"; then
    source_ref="${digest}"
  fi
  current_digest="$(env_value "${ENV_FILE}" SUB2API_IMAGE)" \
    || die "A 容灾环境文件缺少 SUB2API_IMAGE"
  printf 'B 当前权威应用镜像：%s；A 恢复配置：%s\n' "${digest}" "${current_digest}"

  if [ "${dry_run}" = "true" ]; then
    if [ "${current_digest}" = "${digest}" ] && image_is_cached "${digest}"; then
      printf 'dry-run：A 已缓存并配置 B 的权威应用镜像。\n'
    else
      printf 'dry-run：实际确认后将在停止 A 旧容器前拉取并写入 B 的权威应用 digest。\n'
    fi
    return 0
  fi

  docker pull "${digest}" >/dev/null
  local_image_id="$(docker image inspect --format '{{.Id}}' "${digest}")"
  remote_image_id="$(ssh_b "docker image inspect --format '{{.Id}}' '${digest}'")"
  [ "${local_image_id}" = "${remote_image_id}" ] \
    || die "A 拉取镜像后的 image ID与 B 权威镜像不一致"
  set_local_release_image "${digest}" "${source_ref}"
  [ "$(env_value "${ENV_FILE}" SUB2API_IMAGE)" = "${digest}" ] \
    || die "A 恢复环境文件镜像更新后校验失败"
}

verify_recovery_image_matches_b() {
  local remote_output="$1"
  local digest recovery_digest local_release

  digest="$(validate_b_release_image "${remote_output}")"
  recovery_digest="$(env_value "${ENV_FILE}" SUB2API_IMAGE)" \
    || die "A 容灾环境文件缺少 SUB2API_IMAGE"
  local_release="$(release_state_value APP_IMAGE_DIGEST 2>/dev/null || true)"
  [ "${recovery_digest}" = "${digest}" ] \
    || die "A 恢复应用镜像与 B 权威镜像不一致，请先重新执行 prepare-from-b"
  [ "${local_release}" = "${digest}" ] \
    || die "A 发布同步记录与 B 权威镜像不一致"
  image_is_cached "${digest}" || die "A 尚未缓存 B 权威应用镜像：${digest}"
}

volume_is_empty() {
  local volume="$1"

  docker run --rm \
    --entrypoint /bin/sh \
    -v "${volume}:/target" \
    "${POSTGRES_IMAGE}" \
    -ec '[ -z "$(find /target -mindepth 1 -maxdepth 1 -print -quit)" ]'
}

ensure_recovery_volumes_available() {
  local volume

  for volume in \
    "${A_RECOVERY_APP_VOLUME}" \
    "${A_RECOVERY_POSTGRES_VOLUME}" \
    "${A_RECOVERY_REDIS_VOLUME}"; do
    if docker volume inspect "${volume}" >/dev/null 2>&1; then
      volume_is_empty "${volume}" || die "恢复卷 ${volume} 已有数据，拒绝覆盖；请先检查上次执行状态"
    fi
  done
}

wait_for_redis_standby() {
  local attempts="${1:-90}"
  local replication role link sync

  for ((i = 1; i <= attempts; i++)); do
    if [ -n "${REDIS_PASSWORD:-}" ]; then
      replication="$(docker exec -e REDISCLI_AUTH="${REDIS_PASSWORD}" sub2api-redis redis-cli INFO replication 2>/dev/null | tr -d '\r' || true)"
    else
      replication="$(docker exec sub2api-redis sh -lc 'unset REDISCLI_AUTH; redis-cli INFO replication' 2>/dev/null | tr -d '\r' || true)"
    fi
    role="$(printf '%s\n' "${replication}" | awk -F: '$1 == "role" { print $2 }')"
    link="$(printf '%s\n' "${replication}" | awk -F: '$1 == "master_link_status" { print $2 }')"
    sync="$(printf '%s\n' "${replication}" | awk -F: '$1 == "master_sync_in_progress" { print $2 }')"
    if [ "${role}" = "slave" ] && [ "${link}" = "up" ] && [ "${sync}" = "0" ]; then
      return 0
    fi
    sleep 2
  done

  die "A Redis未在预期时间内完成从 B 的同步"
}

verify_b_recovery_source() {
  local replication_slot

  docker run --rm \
    --network host \
    --entrypoint psql \
    -e PGPASSWORD="${POSTGRES_REPLICATION_PASSWORD}" \
    "${POSTGRES_IMAGE}" \
    "host=${B_REPLICATION_HOST} port=${B_POSTGRES_RECOVERY_PORT} user=${POSTGRES_REPLICATION_USER} dbname=postgres replication=database" \
    -Atc IDENTIFY_SYSTEM >/dev/null
  replication_slot="$(docker run --rm \
    --network host \
    --entrypoint psql \
    -e PGPASSWORD="${POSTGRES_REPLICATION_PASSWORD}" \
    "${POSTGRES_IMAGE}" \
    "host=${B_REPLICATION_HOST} port=${B_POSTGRES_RECOVERY_PORT} user=${POSTGRES_REPLICATION_USER} dbname=postgres replication=database" \
    -Atc "READ_REPLICATION_SLOT ${POSTGRES_A_RECOVERY_SLOT}")"
  [[ "${replication_slot}" == physical\|* ]] || die "B 上的 A 恢复物理槽不可用"

  if [ -n "${REDIS_PASSWORD:-}" ]; then
    docker run --rm \
      --network host \
      --entrypoint redis-cli \
      -e REDISCLI_AUTH="${REDIS_PASSWORD}" \
      "${REDIS_IMAGE}" \
      -h "${B_REPLICATION_HOST}" \
      -p "${B_REDIS_RECOVERY_PORT}" ping | grep -qx PONG
  else
    docker run --rm \
      --network host \
      --entrypoint redis-cli \
      "${REDIS_IMAGE}" \
      -h "${B_REPLICATION_HOST}" \
      -p "${B_REDIS_RECOVERY_PORT}" ping | grep -qx PONG
  fi
}

prepare_from_b() {
  local dry_run="$1"
  local local_output remote_output local_mode b_mode confirmation

  local_output="$(local_state)"
  local_mode="$(field_value "${local_output}" mode)"
  case "${local_mode}" in
    legacy-active|offline) ;;
    standby-from-b)
      printf 'A 已经处于 standby-from-b，无需重复初始化。\n'
      return 0
      ;;
    *) die "A 当前模式为 ${local_mode}，拒绝从 B 重建" ;;
  esac

  remote_output="$(b_state)"
  b_mode="$(field_value "${remote_output}" mode)"
  case "${b_mode}" in
    active|active-stopped) ;;
    *) die "B 当前模式为 ${b_mode}，不是可用恢复主节点" ;;
  esac
  validate_b_release_image "${remote_output}" >/dev/null
  ssh_b "cd '${B_DR_ROOT}' && ./scripts/prepare-recovery-source.sh --dry-run"
  ensure_recovery_volumes_available

  if [ "${dry_run}" = "true" ]; then
    ensure_recovery_image_from_b "${remote_output}" true
    printf 'dry-run：将停止并移除 A 旧容器但保留旧卷，随后从 B 初始化三个新恢复卷。\n'
    printf 'dry-run 完成：没有停止服务、创建卷或启动 B 临时出口。\n'
    return 0
  fi

  printf '%s\n' '警告：A 旧容器将停止并移除，新恢复卷会以 B 为权威数据源；故障前旧卷不会删除。'
  read -r -p '请输入 STOP_AND_REBUILD_A_FROM_B 继续：' confirmation
  [ "${confirmation}" = "STOP_AND_REBUILD_A_FROM_B" ] || die "确认口令不匹配，已取消"

  ensure_recovery_image_from_b "${remote_output}" false
  ssh_b "cd '${B_DR_ROOT}' && ./scripts/prepare-recovery-source.sh"
  verify_b_recovery_source
  compose_primary stop sub2api postgres redis
  compose_primary rm -f -s sub2api postgres redis

  "${SCRIPT_DIR}/sync-app-data.sh" from-b
  "${SCRIPT_DIR}/init-postgres-from-b.sh"
  recovery_volume_create "${A_RECOVERY_REDIS_VOLUME}" recovery-redis-data
  volume_is_empty "${A_RECOVERY_REDIS_VOLUME}" || die "A 恢复 Redis卷非空，拒绝覆盖"
  compose_recovery --profile cutback up -d --no-deps redis
  wait_for_healthy sub2api-redis 60
  wait_for_redis_standby 90

  local_output="$(local_state)"
  [ "$(field_value "${local_output}" mode)" = "standby-from-b" ] \
    || die "A 初始化完成后未识别为 standby-from-b"
  write_state a-standby-from-b-ready
  printf 'A 已从 B 重建并持续追平；A 应用保持停止，尚未回切流量。\n'
}

wait_for_final_catchup() {
  local target_lsn="$1"
  local target_redis_offset="$2"
  local attempts="${CUTBACK_WAIT_ATTEMPTS:-120}"
  local replay_ok local_output redis_role redis_link redis_sync local_redis_offset

  [[ "${target_lsn}" =~ ^[0-9A-F]+/[0-9A-F]+$ ]] || die "B PostgreSQL LSN格式无效：${target_lsn}"
  [[ "${target_redis_offset}" =~ ^[0-9]+$ ]] || die "B Redis offset格式无效：${target_redis_offset}"

  for ((i = 1; i <= attempts; i++)); do
    replay_ok="$(docker exec -e PGPASSWORD="${POSTGRES_PASSWORD}" sub2api-postgres \
      psql -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" -Atc \
      "SELECT COALESCE(pg_last_wal_replay_lsn() >= '${target_lsn}'::pg_lsn, false);")"
    local_output="$(local_state)"
    redis_role="$(field_value "${local_output}" redis_role)"
    redis_link="$(field_value "${local_output}" redis_link)"
    redis_sync="$(field_value "${local_output}" redis_sync)"
    local_redis_offset="$(field_value "${local_output}" redis_slave_offset)"
    if [ "${replay_ok}" = "t" ] \
      && [ "${redis_role}" = "slave" ] \
      && [ "${redis_link}" = "up" ] \
      && [ "${redis_sync}" = "0" ] \
      && [[ "${local_redis_offset}" =~ ^[0-9]+$ ]] \
      && ((local_redis_offset >= target_redis_offset)); then
      return 0
    fi
    sleep 2
  done

  die "B 写入冻结后，A 未在预期时间内完全追平 PostgreSQL和 Redis"
}

promote_redis() {
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    docker exec -e REDISCLI_AUTH="${REDIS_PASSWORD}" sub2api-redis redis-cli REPLICAOF NO ONE >/dev/null
  else
    docker exec sub2api-redis sh -lc 'unset REDISCLI_AUTH; redis-cli REPLICAOF NO ONE' >/dev/null
  fi
  compose_recovery_promoted --profile cutback up -d --no-deps --force-recreate redis
  wait_for_healthy sub2api-redis 60
}

cutback_to_a() {
  local dry_run="$1"
  local local_output remote_output local_mode b_mode confirmation target_lsn target_redis_offset

  local_output="$(local_state)"
  local_mode="$(field_value "${local_output}" mode)"
  if [ "${local_mode}" = "active-recovered" ]; then
    printf 'A 已经是恢复后的启用主节点，无需重复提升。\n'
    return 0
  fi
  case "${local_mode}" in
    standby-from-b|cutback-postgres-promoted|active-recovered-stopped) ;;
    *) die "A 当前模式为 ${local_mode}，不允许回切" ;;
  esac

  remote_output="$(b_state)"
  b_mode="$(field_value "${remote_output}" mode)"
  case "${b_mode}" in
    active|active-stopped) ;;
    *) die "B 当前模式为 ${b_mode}，不能执行受控回切" ;;
  esac
  verify_recovery_image_matches_b "${remote_output}"

  if [ "${dry_run}" = "true" ]; then
    ssh_b "cd '${B_DR_ROOT}' && ./scripts/switch-mode.sh freeze --dry-run"
    printf 'dry-run：将冻结 B 应用写入，等待 A 的 PostgreSQL LSN和 Redis offset完全追平，再提升 A。\n'
    printf 'dry-run 完成：没有停止 B 应用、提升数据库或启动 A 应用。\n'
    return 0
  fi

  printf '%s\n' '警告：该阶段会冻结 B 写入并提升 A；入口切回 A 前不得重新启动 B 应用。'
  read -r -p '请输入 FREEZE_B_AND_PROMOTE_A 继续：' confirmation
  [ "${confirmation}" = "FREEZE_B_AND_PROMOTE_A" ] || die "确认口令不匹配，已取消"

  ssh_b_tty "cd '${B_DR_ROOT}' && ./scripts/switch-mode.sh freeze"
  remote_output="$(b_state)"
  [ "$(field_value "${remote_output}" mode)" = "active-stopped" ] \
    || die "B 写入冻结后未识别为 active-stopped"

  if [ "${local_mode}" = "standby-from-b" ]; then
    target_lsn="$(field_value "${remote_output}" postgres_lsn)"
    target_redis_offset="$(field_value "${remote_output}" redis_master_offset)"
    wait_for_final_catchup "${target_lsn}" "${target_redis_offset}"

    docker exec -u postgres sub2api-postgres \
      pg_ctl -D /var/lib/postgresql/data promote -w -t 60
    write_state a-postgres-promoted
  fi

  if [ "${local_mode}" = "standby-from-b" ] \
    || [ "${local_mode}" = "cutback-postgres-promoted" ] \
    || [ "${local_mode}" = "active-recovered-stopped" ]; then
    promote_redis
    write_state a-databases-promoted
  fi

  compose_recovery_promoted --profile cutback up -d --no-deps sub2api
  wait_for_healthy sub2api 90
  curl -fsS "http://127.0.0.1:${SERVER_PORT:-8080}/health" >/dev/null \
    || die "A 应用健康检查失败"
  local_output="$(local_state)"
  [ "$(field_value "${local_output}" mode)" = "active-recovered" ] \
    || die "A 应用启动后未识别为 active-recovered"
  write_state a-cutback-ready
  printf 'A 数据库和应用已恢复为主节点。公共入口尚未自动切换，请人工将流量切回 A 后再重建 B 备库。\n'
}

restore_b_standby() {
  local dry_run="$1"
  local local_output remote_output local_mode b_mode confirmation

  local_output="$(local_state)"
  local_mode="$(field_value "${local_output}" mode)"
  [ "${local_mode}" = "active-recovered" ] \
    || die "A 当前模式为 ${local_mode}，只有恢复后的启用主节点才能重建 B"
  remote_output="$(b_state)"
  b_mode="$(field_value "${remote_output}" mode)"
  if [ "${b_mode}" = "standby" ]; then
    if [ "${dry_run}" = "true" ]; then
      "${SCRIPT_DIR}/sync-app-data.sh" to-b --dry-run
      printf 'dry-run：B 已是备库，只需补齐 A 到 B 的非日志应用数据同步。\n'
      return 0
    fi
    printf '%s\n' 'B 已恢复为备库，将只补齐 A 到 B 的非日志应用数据同步。'
    read -r -p '请输入 SYNC_A_APP_DATA_TO_B 继续：' confirmation
    [ "${confirmation}" = "SYNC_A_APP_DATA_TO_B" ] || die "确认口令不匹配，已取消"
    "${SCRIPT_DIR}/sync-app-data.sh" to-b
    write_state topology-restored-a-primary-b-standby
    printf 'A 主、B 备拓扑已恢复，B 应用数据同步完成。\n'
    return 0
  fi
  [ "${b_mode}" = "active-stopped" ] \
    || die "B 当前模式为 ${b_mode}，必须先冻结 B 应用写入"
  ssh_b "test -x '${B_DR_ROOT}/scripts/restore-standby-from-a.sh'" \
    || die "B 恢复备库脚本不存在或不可执行"

  if [ "${dry_run}" = "true" ]; then
    printf 'dry-run：实际确认后将先校准 A 复制出口和 B 物理槽，再验证并重建 B 容灾数据库卷。\n'
    printf 'dry-run 完成：没有删除 B 卷、改变复制方向或同步应用数据。\n'
    return 0
  fi

  printf '%s\n' '警告：确认公共入口已切回 A。下一步会删除 B 容灾 PostgreSQL/Redis卷并从 A 全量重建。'
  read -r -p '请输入 A_IS_PRIMARY_REBUILD_B_STANDBY 继续：' confirmation
  [ "${confirmation}" = "A_IS_PRIMARY_REBUILD_B_STANDBY" ] || die "确认口令不匹配，已取消"

  compose_export up -d
  wait_for_healthy sub2api-ha-postgres-export 30
  wait_for_healthy sub2api-ha-redis-export 30
  # pg_basebackup 不复制物理复制槽，新 A 必须先重建槽，B 才能验证恢复源。
  "${SCRIPT_DIR}/configure-primary.sh"
  ssh_b "cd '${B_DR_ROOT}' && ./scripts/restore-standby-from-a.sh --dry-run"
  ssh_b_tty "cd '${B_DR_ROOT}' && ./scripts/restore-standby-from-a.sh"
  "${SCRIPT_DIR}/sync-app-data.sh" to-b

  remote_output="$(b_state)"
  [ "$(field_value "${remote_output}" mode)" = "standby" ] \
    || die "B 重建后未识别为 standby"
  write_state topology-restored-a-primary-b-standby
  printf 'A 主、B 备拓扑已恢复。请继续监控 PostgreSQL复制槽积压、Redis offset和 B 时间同步。\n'
}

main() {
  local action="${1:-status}"
  local option="${2:-}"
  local dry_run=false
  local machine=false

  [ "$#" -le 2 ] || die "参数过多；请使用 --help 查看用法"
  case "${action}" in
    -h|--help|help)
      [ -z "${option}" ] || die "帮助命令不接受额外参数"
      usage
      return 0
      ;;
  esac
  case "${action}:${option}" in
    status:) ;;
    status:--machine) machine=true ;;
    sync-release:|prepare-from-b:|cutback-to-a:|restore-b-standby:) ;;
    sync-release:--dry-run|prepare-from-b:--dry-run|cutback-to-a:--dry-run|restore-b-standby:--dry-run) dry_run=true ;;
    *) die "操作 ${action} 不支持参数：${option:-<空>}" ;;
  esac

  require_command awk
  require_command curl
  require_command docker
  require_command mktemp
  require_command ssh
  load_recovery_env
  POSTGRES_USER="${POSTGRES_USER:-sub2api}"
  POSTGRES_DB="${POSTGRES_DB:-sub2api}"
  require_var POSTGRES_PASSWORD
  require_var POSTGRES_IMAGE
  require_var REDIS_IMAGE
  require_var POSTGRES_REPLICATION_USER
  require_var POSTGRES_REPLICATION_PASSWORD
  require_var POSTGRES_A_RECOVERY_SLOT
  require_var B_REPLICATION_HOST
  require_var B_POSTGRES_RECOVERY_PORT
  require_var B_REDIS_RECOVERY_PORT
  require_var A_RECOVERY_APP_VOLUME
  require_var A_RECOVERY_POSTGRES_VOLUME
  require_var A_RECOVERY_REDIS_VOLUME

  case "${action}" in
    status) show_status "${machine}" ;;
    sync-release) sync_release "${dry_run}" ;;
    prepare-from-b) prepare_from_b "${dry_run}" ;;
    cutback-to-a) cutback_to_a "${dry_run}" ;;
    restore-b-standby) restore_b_standby "${dry_run}" ;;
    *)
      usage >&2
      die "未知操作：${action}"
      ;;
  esac
}

main "$@"
