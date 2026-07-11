#!/usr/bin/env bash

set -Eeuo pipefail

TEST_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
TASK_DIR="$(cd -- "${TEST_DIR}/.." && pwd)"
A_SOURCE="${TASK_DIR}/artifacts/sub2api-ha-export"
B_SOURCE="${TASK_DIR}/artifacts/sub2api-dr"
MOCK_BIN="${TEST_DIR}/mocks"
ORIGINAL_PATH="${PATH}"
TEST_ROOT="$(mktemp -d)"
PASS_COUNT=0
FAIL_COUNT=0

REPOSITORY=ghcr.io/silentflower/sub2api
SOURCE_REF="${REPOSITORY}:build-latest"
OLD_DIGEST="${REPOSITORY}@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
NEW_DIGEST="${REPOSITORY}@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
ALT_DIGEST="${REPOSITORY}@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
WRONG_DIGEST="ghcr.io/other/sub2api@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
OLD_IMAGE_ID=sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
NEW_IMAGE_ID=sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee

cleanup() {
  rm -rf "${TEST_ROOT}"
}
trap cleanup EXIT

fail_assertion() {
  printf '断言失败：%s\n' "$*" >&2
  return 1
}

assert_equals() {
  local expected="$1"
  local actual="$2"
  local label="$3"

  [ "${actual}" = "${expected}" ] \
    || fail_assertion "${label}，期望=${expected}，实际=${actual}"
}

assert_contains() {
  local file="$1"
  local expected="$2"

  grep -Fq -- "${expected}" "${file}" \
    || fail_assertion "${file} 未包含：${expected}"
}

assert_not_contains() {
  local file="$1"
  local unexpected="$2"

  if grep -Fq -- "${unexpected}" "${file}"; then
    fail_assertion "${file} 不应包含：${unexpected}"
  fi
}

assert_hash_unchanged() {
  local file="$1"
  local before="$2"
  local after

  after="$(sha256sum "${file}" | awk '{ print $1 }')"
  assert_equals "${before}" "${after}" "${file} 哈希"
}

expect_failure() {
  local output_file="$1"
  shift

  if "$@" >"${output_file}" 2>&1; then
    fail_assertion "命令应失败但返回成功：$*"
  fi
}

run_test() {
  local name="$1"
  shift

  if ("$@"); then
    PASS_COUNT=$((PASS_COUNT + 1))
    printf 'ok %d - %s\n' "${PASS_COUNT}" "${name}"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf 'not ok - %s\n' "${name}" >&2
  fi
}

create_image_map() {
  local file="$1"

  {
    printf '%s\t%s\n' "${OLD_DIGEST}" "${OLD_IMAGE_ID}"
    printf '%s\t%s\n' "${NEW_DIGEST}" "${NEW_IMAGE_ID}"
    printf '%s\t%s\n' "${ALT_DIGEST}" "${NEW_IMAGE_ID}"
    printf '%s\t%s\n' "${WRONG_DIGEST}" "${NEW_IMAGE_ID}"
  } >"${file}"
}

configure_common_mocks() {
  local case_dir="$1"

  export PATH="${MOCK_BIN}:${ORIGINAL_PATH}"
  export MOCK_IMAGE_MAP_FILE="${case_dir}/image-map.tsv"
  export MOCK_SSH_LOG="${case_dir}/ssh.log"
  export MOCK_DOCKER_LOG="${case_dir}/docker.log"
  export MOCK_A_SOURCE_REF="${SOURCE_REF}"
  export MOCK_A_IMAGE_ID="${NEW_IMAGE_ID}"
  export MOCK_REPO_DIGESTS="${NEW_DIGEST}"
  export MOCK_REMOTE_IMAGE_ID="${NEW_IMAGE_ID}"
  export MOCK_APP_STATE=absent
  export MOCK_PG_RECOVERY=t
  export MOCK_PG_LSN=1/100
  export MOCK_REDIS_ROLE=slave
  export MOCK_REDIS_LINK=up
  export MOCK_REDIS_SYNC=0
  export MOCK_REDIS_MASTER_OFFSET=100
  export MOCK_REDIS_SLAVE_OFFSET=100
  export MOCK_CACHE_MISSING_REF=
  export MOCK_SSH_FAIL=none
  export MOCK_VERIFY_REPLICATION_FAIL=false
  export MOCK_CURL_FAIL=false
  export MOCK_B_ROOT=
  export MOCK_A_DYNAMIC_ROOT=
  export MOCK_A_RUNNING_DIGEST=
  : >"${MOCK_SSH_LOG}"
  : >"${MOCK_DOCKER_LOG}"
  create_image_map "${MOCK_IMAGE_MAP_FILE}"
}

write_release_state() {
  local file="$1"
  local digest="$2"
  local previous="$3"

  cat >"${file}" <<EOF
APP_IMAGE_DIGEST=${digest}
PREVIOUS_APP_IMAGE_DIGEST=${previous}
SOURCE_IMAGE_REF=${SOURCE_REF}
SYNCED_AT=2026-07-11T12:00:00+08:00
EOF
  chmod 0600 "${file}"
}

write_a_status() {
  local file="$1"
  local mode="$2"
  local app_digest="$3"
  local recovery_digest="$4"
  local release_digest="$5"

  cat >"${file}" <<EOF
mode=${mode}
postgres_container=running
postgres_recovery=f
postgres_lsn=1/100
postgres_volume=legacy-postgres
redis_container=running
redis_role=master
redis_link=
redis_sync=
redis_master_offset=100
redis_slave_offset=
redis_volume=legacy-redis
app_container=running
app_volume=legacy-app
app_image_digest=${app_digest}
recovery_image_digest=${recovery_digest}
recovery_image_cached=yes
release_image_digest=${release_digest}
release_source_ref=${SOURCE_REF}
release_synced_at=2026-07-11T12:00:00+08:00
EOF
}

write_b_status() {
  local file="$1"
  local mode="$2"
  local digest="$3"
  local release_digest="$4"
  local cached="${5:-yes}"
  local postgres_recovery=t
  local redis_role=slave
  local app_container=absent
  local running_digest=absent

  if [ "${mode}" = "active" ] || [ "${mode}" = "active-stopped" ]; then
    postgres_recovery=f
    redis_role=master
  fi
  if [ "${mode}" = "active" ]; then
    app_container=running
    running_digest="${digest}"
  fi

  cat >"${file}" <<EOF
mode=${mode}
postgres_container=running
postgres_recovery=${postgres_recovery}
postgres_lsn=1/100
redis_container=running
redis_role=${redis_role}
redis_link=$([ "${redis_role}" = slave ] && printf up)
redis_sync=$([ "${redis_role}" = slave ] && printf 0)
redis_master_offset=100
redis_slave_offset=$([ "${redis_role}" = slave ] && printf 100)
app_container=${app_container}
app_image_digest=${digest}
app_image_cached=${cached}
running_app_image_digest=${running_digest}
release_image_digest=${release_digest}
release_source_ref=${SOURCE_REF}
release_synced_at=2026-07-11T12:00:00+08:00
EOF
}

create_a_fixture() {
  local root="$1"

  mkdir -p "${root}/scripts" "${root}/state"
  cp "${A_SOURCE}/scripts/lib.sh" "${root}/scripts/lib.sh"
  cp "${A_SOURCE}/scripts/switch-mode.sh" "${root}/scripts/switch-mode.sh"
  cat >"${root}/scripts/verify-cutback.sh" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
if [ -z "${MOCK_A_DYNAMIC_ROOT:-}" ]; then
  cat "${MOCK_A_STATUS_FILE:?}"
  exit 0
fi
recovery_digest="$(awk -F= '$1 == "SUB2API_IMAGE" { print substr($0, index($0, "=") + 1) }' "${MOCK_A_DYNAMIC_ROOT}/.env")"
release_digest="$(awk -F= '$1 == "APP_IMAGE_DIGEST" { print substr($0, index($0, "=") + 1) }' "${MOCK_A_DYNAMIC_ROOT}/state/release-image.env")"
cat <<STATUS
mode=legacy-active
postgres_container=running
postgres_recovery=f
postgres_lsn=1/100
postgres_volume=legacy-postgres
redis_container=running
redis_role=master
redis_link=
redis_sync=
redis_master_offset=100
redis_slave_offset=
redis_volume=legacy-redis
app_container=running
app_volume=legacy-app
app_image_digest=${MOCK_A_RUNNING_DIGEST:?}
recovery_image_digest=${recovery_digest}
recovery_image_cached=yes
release_image_digest=${release_digest}
release_source_ref=${MOCK_A_SOURCE_REF:?}
release_synced_at=2026-07-11T12:00:00+08:00
STATUS
EOF
  chmod 0755 "${root}/scripts/verify-cutback.sh" "${root}/scripts/switch-mode.sh"

  : >"${root}/primary-compose.yaml"
  cat >"${root}/primary.env" <<'EOF'
POSTGRES_USER=sub2api
POSTGRES_PASSWORD=test-postgres
POSTGRES_DB=sub2api
REDIS_PASSWORD=test-redis
SERVER_PORT=8080
EOF
  cat >"${root}/.env" <<EOF
PRIMARY_PROJECT_NAME=deploy
PRIMARY_COMPOSE_FILE=${root}/primary-compose.yaml
PRIMARY_ENV_FILE=${root}/primary.env
B_SSH_TARGET=root@b.test
B_DR_ROOT=/root/sub2api-dr
SSH_CONNECT_TIMEOUT=1
SUB2API_IMAGE=${OLD_DIGEST}
POSTGRES_IMAGE=postgres:test
REDIS_IMAGE=redis:test
B_REPLICATION_HOST=192.0.2.2
B_POSTGRES_RECOVERY_PORT=25432
B_REDIS_RECOVERY_PORT=26379
A_RECOVERY_APP_VOLUME=test-a-app
A_RECOVERY_POSTGRES_VOLUME=test-a-postgres
A_RECOVERY_REDIS_VOLUME=test-a-redis
EOF
  cat >"${root}/secrets.env" <<'EOF'
POSTGRES_REPLICATION_USER=replicator
POSTGRES_REPLICATION_PASSWORD=test-replication
POSTGRES_A_RECOVERY_SLOT=test_a_recovery
EOF
  chmod 0600 "${root}/.env" "${root}/primary.env" "${root}/secrets.env"
  write_release_state "${root}/state/release-image.env" "${OLD_DIGEST}" ""
}

create_b_fixture() {
  local root="$1"

  mkdir -p "${root}/scripts" "${root}/state"
  cp "${B_SOURCE}/scripts/lib.sh" "${root}/scripts/lib.sh"
  cp "${B_SOURCE}/scripts/prepare-runtime-env.sh" "${root}/scripts/prepare-runtime-env.sh"
  cp "${B_SOURCE}/scripts/switch-mode.sh" "${root}/scripts/switch-mode.sh"
  cp "${B_SOURCE}/scripts/set-release-image.sh" "${root}/scripts/set-release-image.sh"
  cat >"${root}/scripts/verify-replication.sh" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
[ "${MOCK_VERIFY_REPLICATION_FAIL:-false}" != "true" ]
EOF
  cat >"${root}/scripts/promote.sh" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf '%s\n' "$*" >> "${MOCK_PROMOTE_LOG:?}"
EOF
  chmod 0755 "${root}/scripts/"*.sh
  cat >"${root}/.env" <<EOF
POSTGRES_USER=sub2api
POSTGRES_PASSWORD=test-postgres
POSTGRES_DB=sub2api
SUB2API_IMAGE=${OLD_DIGEST}
OTHER_SETTING=preserve-me
EOF
  chmod 0600 "${root}/.env"
  write_release_state "${root}/state/release-image.env" "${OLD_DIGEST}" ""
}

test_resolves_new_digest_from_reused_tag() {
  local case_dir="${TEST_ROOT}/resolve-new"
  local actual

  mkdir -p "${case_dir}"
  configure_common_mocks "${case_dir}"
  actual="$(
    # shellcheck source=/dev/null
    . "${A_SOURCE}/scripts/lib.sh"
    resolve_running_image_digest sub2api
  )"
  assert_equals "${NEW_DIGEST}" "${actual}" "同标签运行镜像 digest"
}

test_rejects_missing_and_wrong_repository_digest() {
  local case_dir="${TEST_ROOT}/resolve-invalid"
  local output="${case_dir}/output.log"

  mkdir -p "${case_dir}"
  configure_common_mocks "${case_dir}"
  export MOCK_REPO_DIGESTS=
  expect_failure "${output}" bash -c '. "$1"; resolve_running_image_digest sub2api' _ \
    "${A_SOURCE}/scripts/lib.sh"
  assert_contains "${output}" "没有与仓库 ${REPOSITORY} 匹配"

  export MOCK_REPO_DIGESTS="${WRONG_DIGEST}"
  expect_failure "${output}" bash -c '. "$1"; resolve_running_image_digest sub2api' _ \
    "${A_SOURCE}/scripts/lib.sh"
  assert_contains "${output}" "没有与仓库 ${REPOSITORY} 匹配"

  {
    printf '%s\t%s\n' "${NEW_DIGEST}" "${OLD_IMAGE_ID}"
  } >"${MOCK_IMAGE_MAP_FILE}"
  export MOCK_REPO_DIGESTS="${NEW_DIGEST}"
  expect_failure "${output}" bash -c '. "$1"; resolve_running_image_digest sub2api' _ \
    "${A_SOURCE}/scripts/lib.sh"
  assert_contains "${output}" "没有与仓库 ${REPOSITORY} 匹配"

  create_image_map "${MOCK_IMAGE_MAP_FILE}"
  export MOCK_REPO_DIGESTS="${NEW_DIGEST}"$'\n'"${ALT_DIGEST}"
  expect_failure "${output}" bash -c '. "$1"; resolve_running_image_digest sub2api' _ \
    "${A_SOURCE}/scripts/lib.sh"
  assert_contains "${output}" "匹配到多个同仓库 digest"
}

test_b_updates_only_release_image() {
  local case_dir="${TEST_ROOT}/b-update"
  local root="${case_dir}/b"
  local output="${case_dir}/output.log"

  mkdir -p "${case_dir}"
  configure_common_mocks "${case_dir}"
  create_b_fixture "${root}"
  export MOCK_PROMOTE_LOG="${case_dir}/promote.log"

  "${root}/scripts/set-release-image.sh" "${NEW_DIGEST}" "${SOURCE_REF}" >"${output}"
  assert_equals "1" "$(grep -c '^SUB2API_IMAGE=' "${root}/.env")" "B SUB2API_IMAGE 数量"
  assert_contains "${root}/.env" "SUB2API_IMAGE=${NEW_DIGEST}"
  assert_contains "${root}/.env" "OTHER_SETTING=preserve-me"
  assert_contains "${root}/state/release-image.env" "APP_IMAGE_DIGEST=${NEW_DIGEST}"
  assert_contains "${root}/state/release-image.env" "PREVIOUS_APP_IMAGE_DIGEST=${OLD_DIGEST}"
  assert_equals "600" "$(stat -c '%a' "${root}/.env")" "B .env 权限"
  assert_equals "600" "$(stat -c '%a' "${root}/state/release-image.env")" "B 发布状态权限"
}

test_prepare_runtime_env_removes_duplicate_override_keys() {
  local case_dir="${TEST_ROOT}/prepare-runtime-env"
  local root="${case_dir}/b"
  local source_env="${case_dir}/source.env"
  local secrets="${case_dir}/secrets.env"
  local postgres_digest="postgres@sha256:1111111111111111111111111111111111111111111111111111111111111111"
  local redis_digest="redis@sha256:2222222222222222222222222222222222222222222222222222222222222222"
  local key

  mkdir -p "${case_dir}"
  configure_common_mocks "${case_dir}"
  create_b_fixture "${root}"
  cat >"${source_env}" <<EOF
POSTGRES_USER=sub2api
POSTGRES_PASSWORD=test-postgres
POSTGRES_DB=sub2api
REDIS_PASSWORD=
JWT_SECRET=
OTHER_SETTING=preserve-me
COMPOSE_PROJECT_NAME=old-project
SUB2API_IMAGE=${OLD_DIGEST}
SUB2API_IMAGE=${WRONG_DIGEST}
A_REPLICATION_HOST=198.51.100.10
POSTGRES_REPLICATION_SLOT=old-slot
EOF
  cat >"${secrets}" <<'EOF'
POSTGRES_REPLICATION_PASSWORD=test-replication
EOF

  A_REPLICATION_HOST=192.0.2.10 \
  SUB2API_IMAGE="${NEW_DIGEST}" \
  POSTGRES_IMAGE="${postgres_digest}" \
  REDIS_IMAGE="${redis_digest}" \
  POSTGRES_REPLICATION_USER=replicator \
  POSTGRES_REPLICATION_SLOT=test_b_standby \
    "${root}/scripts/prepare-runtime-env.sh" "${source_env}" "${secrets}" >/dev/null

  for key in \
    COMPOSE_PROJECT_NAME DR_BIND_HOST DR_APP_PORT SUB2API_IMAGE POSTGRES_IMAGE REDIS_IMAGE \
    A_REPLICATION_HOST A_POSTGRES_REPLICATION_PORT POSTGRES_REPLICATION_USER \
    POSTGRES_REPLICATION_PASSWORD POSTGRES_REPLICATION_SLOT REDIS_MASTER_PASSWORD \
    A_REDIS_REPLICATION_PORT; do
    assert_equals "1" "$(grep -c "^${key}=" "${root}/.env")" "${key} 数量"
  done
  assert_contains "${root}/.env" "SUB2API_IMAGE=${NEW_DIGEST}"
  assert_contains "${root}/.env" "OTHER_SETTING=preserve-me"
  assert_not_contains "${root}/.env" "old-project"
  assert_equals "600" "$(stat -c '%a' "${root}/.env")" "生成的 B .env 权限"
}

test_b_rejects_non_standby_and_unhealthy_replication() {
  local case_dir="${TEST_ROOT}/b-role-gates"
  local root="${case_dir}/b"
  local output="${case_dir}/output.log"
  local before

  mkdir -p "${case_dir}"
  configure_common_mocks "${case_dir}"
  create_b_fixture "${root}"
  export MOCK_PROMOTE_LOG="${case_dir}/promote.log"
  export MOCK_PG_RECOVERY=f
  export MOCK_REDIS_ROLE=master
  before="$(sha256sum "${root}/.env" | awk '{ print $1 }')"
  expect_failure "${output}" "${root}/scripts/set-release-image.sh" "${NEW_DIGEST}" "${SOURCE_REF}"
  assert_contains "${output}" "只允许在 standby更新备用应用镜像"
  assert_hash_unchanged "${root}/.env" "${before}"

  export MOCK_PG_RECOVERY=t
  export MOCK_REDIS_ROLE=slave
  export MOCK_VERIFY_REPLICATION_FAIL=true
  expect_failure "${output}" "${root}/scripts/set-release-image.sh" "${NEW_DIGEST}" "${SOURCE_REF}"
  assert_hash_unchanged "${root}/.env" "${before}"
}

test_b_enable_rejects_drift_and_missing_cache() {
  local case_dir="${TEST_ROOT}/b-enable-gates"
  local root="${case_dir}/b"
  local output="${case_dir}/output.log"
  local env_before state_before

  mkdir -p "${case_dir}"
  configure_common_mocks "${case_dir}"
  create_b_fixture "${root}"
  export MOCK_PROMOTE_LOG="${case_dir}/promote.log"
  sed -i "s|^SUB2API_IMAGE=.*|SUB2API_IMAGE=${NEW_DIGEST}|" "${root}/.env"
  env_before="$(sha256sum "${root}/.env" | awk '{ print $1 }')"
  state_before="$(sha256sum "${root}/state/release-image.env" | awk '{ print $1 }')"

  expect_failure "${output}" "${root}/scripts/switch-mode.sh" enable --dry-run
  assert_contains "${output}" "与最近发布同步记录不一致"
  [ ! -e "${MOCK_PROMOTE_LOG}" ] || fail_assertion "漂移时不应进入 promote"
  assert_hash_unchanged "${root}/.env" "${env_before}"
  assert_hash_unchanged "${root}/state/release-image.env" "${state_before}"

  write_release_state "${root}/state/release-image.env" "${NEW_DIGEST}" "${OLD_DIGEST}"
  state_before="$(sha256sum "${root}/state/release-image.env" | awk '{ print $1 }')"
  export MOCK_CACHE_MISSING_REF="${NEW_DIGEST}"
  expect_failure "${output}" "${root}/scripts/switch-mode.sh" enable --dry-run
  assert_contains "${output}" "尚未缓存容灾应用镜像"
  assert_hash_unchanged "${root}/.env" "${env_before}"
  assert_hash_unchanged "${root}/state/release-image.env" "${state_before}"
}

test_b_enable_accepts_synced_digest_without_writes() {
  local case_dir="${TEST_ROOT}/b-enable-ok"
  local root="${case_dir}/b"
  local output="${case_dir}/output.log"
  local env_before state_before

  mkdir -p "${case_dir}"
  configure_common_mocks "${case_dir}"
  create_b_fixture "${root}"
  export MOCK_PROMOTE_LOG="${case_dir}/promote.log"
  sed -i "s|^SUB2API_IMAGE=.*|SUB2API_IMAGE=${NEW_DIGEST}|" "${root}/.env"
  write_release_state "${root}/state/release-image.env" "${NEW_DIGEST}" "${OLD_DIGEST}"
  env_before="$(sha256sum "${root}/.env" | awk '{ print $1 }')"
  state_before="$(sha256sum "${root}/state/release-image.env" | awk '{ print $1 }')"

  "${root}/scripts/switch-mode.sh" enable --dry-run >"${output}"
  assert_contains "${MOCK_PROMOTE_LOG}" "--dry-run"
  assert_contains "${output}" "没有提升数据库或启动应用"
  assert_hash_unchanged "${root}/.env" "${env_before}"
  assert_hash_unchanged "${root}/state/release-image.env" "${state_before}"
}

test_a_sync_dry_run_detects_new_digest_without_writes() {
  local case_dir="${TEST_ROOT}/a-sync-new"
  local root="${case_dir}/a"
  local output="${case_dir}/output.log"
  local env_before state_before

  mkdir -p "${case_dir}"
  configure_common_mocks "${case_dir}"
  create_a_fixture "${root}"
  export MOCK_A_STATUS_FILE="${case_dir}/a-status.env"
  export MOCK_B_STATUS_FILE="${case_dir}/b-status.env"
  write_a_status "${MOCK_A_STATUS_FILE}" legacy-active "${NEW_DIGEST}" "${OLD_DIGEST}" "${OLD_DIGEST}"
  write_b_status "${MOCK_B_STATUS_FILE}" standby "${OLD_DIGEST}" "${OLD_DIGEST}"
  env_before="$(sha256sum "${root}/.env" | awk '{ print $1 }')"
  state_before="$(sha256sum "${root}/state/release-image.env" | awk '{ print $1 }')"

  "${root}/scripts/switch-mode.sh" sync-release --dry-run >"${output}"
  assert_contains "${output}" "source=${SOURCE_REF} digest=${NEW_DIGEST}"
  assert_contains "${output}" "sync=drift"
  assert_contains "${output}" "没有拉取镜像、修改环境文件、重启服务或启动 B 应用"
  assert_contains "${MOCK_SSH_LOG}" "verify-replication.sh"
  assert_not_contains "${MOCK_SSH_LOG}" "docker pull"
  assert_not_contains "${MOCK_SSH_LOG}" "set-release-image.sh"
  assert_hash_unchanged "${root}/.env" "${env_before}"
  assert_hash_unchanged "${root}/state/release-image.env" "${state_before}"
}

test_a_sync_updates_b_then_a_for_reused_tag() {
  local case_dir="${TEST_ROOT}/a-sync-actual"
  local a_root="${case_dir}/a"
  local b_root="${case_dir}/b"
  local output="${case_dir}/output.log"

  mkdir -p "${case_dir}"
  configure_common_mocks "${case_dir}"
  create_a_fixture "${a_root}"
  create_b_fixture "${b_root}"
  export MOCK_A_DYNAMIC_ROOT="${a_root}"
  export MOCK_A_RUNNING_DIGEST="${NEW_DIGEST}"
  export MOCK_B_ROOT="${b_root}"
  export MOCK_PROMOTE_LOG="${case_dir}/promote.log"

  "${a_root}/scripts/switch-mode.sh" sync-release >"${output}"
  assert_contains "${output}" "发布镜像同步完成：${NEW_DIGEST}"
  assert_contains "${a_root}/.env" "SUB2API_IMAGE=${NEW_DIGEST}"
  assert_contains "${b_root}/.env" "SUB2API_IMAGE=${NEW_DIGEST}"
  assert_equals "1" "$(grep -c '^SUB2API_IMAGE=' "${a_root}/.env")" "A SUB2API_IMAGE 数量"
  assert_equals "1" "$(grep -c '^SUB2API_IMAGE=' "${b_root}/.env")" "B SUB2API_IMAGE 数量"
  assert_contains "${a_root}/state/release-image.env" "APP_IMAGE_DIGEST=${NEW_DIGEST}"
  assert_contains "${a_root}/state/release-image.env" "PREVIOUS_APP_IMAGE_DIGEST=${OLD_DIGEST}"
  assert_contains "${b_root}/state/release-image.env" "APP_IMAGE_DIGEST=${NEW_DIGEST}"
  assert_contains "${b_root}/state/release-image.env" "PREVIOUS_APP_IMAGE_DIGEST=${OLD_DIGEST}"
  assert_contains "${MOCK_SSH_LOG}" "docker pull '${NEW_DIGEST}'"
  assert_contains "${MOCK_SSH_LOG}" "set-release-image.sh '${NEW_DIGEST}' '${SOURCE_REF}'"
}

test_a_sync_stops_on_ssh_failure_before_writes() {
  local case_dir="${TEST_ROOT}/a-ssh-failure"
  local root="${case_dir}/a"
  local output="${case_dir}/output.log"
  local env_before state_before

  mkdir -p "${case_dir}"
  configure_common_mocks "${case_dir}"
  create_a_fixture "${root}"
  export MOCK_A_STATUS_FILE="${case_dir}/a-status.env"
  export MOCK_B_STATUS_FILE="${case_dir}/b-status.env"
  export MOCK_SSH_FAIL=all
  write_a_status "${MOCK_A_STATUS_FILE}" legacy-active "${NEW_DIGEST}" "${OLD_DIGEST}" "${OLD_DIGEST}"
  write_b_status "${MOCK_B_STATUS_FILE}" standby "${OLD_DIGEST}" "${OLD_DIGEST}"
  env_before="$(sha256sum "${root}/.env" | awk '{ print $1 }')"
  state_before="$(sha256sum "${root}/state/release-image.env" | awk '{ print $1 }')"

  expect_failure "${output}" "${root}/scripts/switch-mode.sh" sync-release --dry-run
  assert_hash_unchanged "${root}/.env" "${env_before}"
  assert_hash_unchanged "${root}/state/release-image.env" "${state_before}"
  assert_equals "0" "$(wc -l <"${MOCK_DOCKER_LOG}")" "SSH失败后的 Docker 变更调用数"
}

test_a_sync_stops_on_unhealthy_app_before_remote_access() {
  local case_dir="${TEST_ROOT}/a-health-failure"
  local root="${case_dir}/a"
  local output="${case_dir}/output.log"
  local env_before state_before

  mkdir -p "${case_dir}"
  configure_common_mocks "${case_dir}"
  create_a_fixture "${root}"
  export MOCK_A_STATUS_FILE="${case_dir}/a-status.env"
  export MOCK_B_STATUS_FILE="${case_dir}/b-status.env"
  export MOCK_CURL_FAIL=true
  write_a_status "${MOCK_A_STATUS_FILE}" legacy-active "${NEW_DIGEST}" "${OLD_DIGEST}" "${OLD_DIGEST}"
  write_b_status "${MOCK_B_STATUS_FILE}" standby "${OLD_DIGEST}" "${OLD_DIGEST}"
  env_before="$(sha256sum "${root}/.env" | awk '{ print $1 }')"
  state_before="$(sha256sum "${root}/state/release-image.env" | awk '{ print $1 }')"

  expect_failure "${output}" "${root}/scripts/switch-mode.sh" sync-release --dry-run
  assert_contains "${output}" "A 应用健康检查失败"
  assert_equals "0" "$(wc -l <"${MOCK_SSH_LOG}")" "健康失败后的 SSH 调用数"
  assert_hash_unchanged "${root}/.env" "${env_before}"
  assert_hash_unchanged "${root}/state/release-image.env" "${state_before}"
}

test_a_status_reports_drift() {
  local case_dir="${TEST_ROOT}/a-status-drift"
  local root="${case_dir}/a"
  local output="${case_dir}/output.log"

  mkdir -p "${case_dir}"
  configure_common_mocks "${case_dir}"
  create_a_fixture "${root}"
  export MOCK_A_STATUS_FILE="${case_dir}/a-status.env"
  export MOCK_B_STATUS_FILE="${case_dir}/b-status.env"
  write_a_status "${MOCK_A_STATUS_FILE}" legacy-active "${OLD_DIGEST}" "${OLD_DIGEST}" "${OLD_DIGEST}"
  write_b_status "${MOCK_B_STATUS_FILE}" standby "${NEW_DIGEST}" "${NEW_DIGEST}"

  "${root}/scripts/switch-mode.sh" status --machine >"${output}"
  assert_contains "${output}" "dr_image_digest=${NEW_DIGEST}"
  assert_contains "${output}" "image_sync=drift"
}

test_a_recovery_uses_b_active_digest_in_dry_run() {
  local case_dir="${TEST_ROOT}/a-recovery-digest"
  local root="${case_dir}/a"
  local output="${case_dir}/output.log"
  local env_before

  mkdir -p "${case_dir}"
  configure_common_mocks "${case_dir}"
  create_a_fixture "${root}"
  export MOCK_A_STATUS_FILE="${case_dir}/a-status.env"
  export MOCK_B_STATUS_FILE="${case_dir}/b-status.env"
  write_a_status "${MOCK_A_STATUS_FILE}" legacy-active "${OLD_DIGEST}" "${OLD_DIGEST}" "${OLD_DIGEST}"
  write_b_status "${MOCK_B_STATUS_FILE}" active-stopped "${NEW_DIGEST}" "${NEW_DIGEST}"
  env_before="$(sha256sum "${root}/.env" | awk '{ print $1 }')"

  "${root}/scripts/switch-mode.sh" prepare-from-b --dry-run >"${output}"
  assert_contains "${output}" "B 当前权威应用镜像：${NEW_DIGEST}"
  assert_contains "${output}" "A 恢复配置：${OLD_DIGEST}"
  assert_contains "${output}" "将在停止 A 旧容器前拉取并写入 B 的权威应用 digest"
  assert_contains "${MOCK_SSH_LOG}" "prepare-recovery-source.sh --dry-run"
  assert_not_contains "${MOCK_SSH_LOG}" "docker pull"
  assert_hash_unchanged "${root}/.env" "${env_before}"
}

test_machine_status_field_contracts() {
  local case_dir="${TEST_ROOT}/status-contracts"
  local a_root="${case_dir}/a"
  local b_root="${case_dir}/b"
  local a_output="${case_dir}/a-output.env"
  local b_output="${case_dir}/b-output.env"
  local a_expected="${case_dir}/a-expected.txt"
  local b_expected="${case_dir}/b-expected.txt"

  mkdir -p "${case_dir}"
  configure_common_mocks "${case_dir}"
  create_a_fixture "${a_root}"
  create_b_fixture "${b_root}"
  export MOCK_A_STATUS_FILE="${case_dir}/a-status.env"
  export MOCK_B_STATUS_FILE="${case_dir}/b-status.env"
  export MOCK_PROMOTE_LOG="${case_dir}/promote.log"
  write_a_status "${MOCK_A_STATUS_FILE}" legacy-active "${OLD_DIGEST}" "${OLD_DIGEST}" "${OLD_DIGEST}"
  write_b_status "${MOCK_B_STATUS_FILE}" standby "${OLD_DIGEST}" "${OLD_DIGEST}"

  "${a_root}/scripts/switch-mode.sh" status --machine >"${a_output}"
  "${b_root}/scripts/switch-mode.sh" status --machine >"${b_output}"
  cut -d= -f1 "${a_output}" >"${case_dir}/a-actual.txt"
  cut -d= -f1 "${b_output}" >"${case_dir}/b-actual.txt"
  cat >"${a_expected}" <<'EOF'
mode
postgres_container
postgres_recovery
postgres_lsn
postgres_volume
redis_container
redis_role
redis_link
redis_sync
redis_master_offset
redis_slave_offset
redis_volume
app_container
app_volume
app_image_digest
recovery_image_digest
recovery_image_cached
release_image_digest
release_source_ref
release_synced_at
dr_image_digest
dr_image_cached
dr_release_image_digest
dr_release_synced_at
image_sync
EOF
  cat >"${b_expected}" <<'EOF'
mode
postgres_container
postgres_recovery
postgres_lsn
redis_container
redis_role
redis_link
redis_sync
redis_master_offset
redis_slave_offset
app_container
app_image_digest
app_image_cached
running_app_image_digest
release_image_digest
release_source_ref
release_synced_at
EOF
  diff -u "${a_expected}" "${case_dir}/a-actual.txt"
  diff -u "${b_expected}" "${case_dir}/b-actual.txt"
}

test_destructive_order_contracts() {
  local a_switch="${A_SOURCE}/scripts/switch-mode.sh"
  local b_switch="${B_SOURCE}/scripts/switch-mode.sh"
  local b_promote="${B_SOURCE}/scripts/promote.sh"
  local sync_pull sync_remote sync_local recovery_image stop_primary enable_gate enable_promote promote_gate promote_status

  sync_pull="$(grep -n 'ssh_b "docker pull' "${a_switch}" | cut -d: -f1)"
  sync_remote="$(grep -n 'set-release-image.sh' "${a_switch}" | head -1 | cut -d: -f1)"
  sync_local="$(grep -n 'set_local_release_image "${digest}"' "${a_switch}" | head -1 | cut -d: -f1)"
  recovery_image="$(grep -n 'ensure_recovery_image_from_b "${remote_output}" false' "${a_switch}" | cut -d: -f1)"
  stop_primary="$(grep -n 'compose_primary stop sub2api postgres redis' "${a_switch}" | cut -d: -f1)"
  enable_gate="$(grep -n 'verify_release_image_ready' "${b_switch}" | head -1 | cut -d: -f1)"
  enable_promote="$(grep -n '"${SCRIPT_DIR}/promote.sh"' "${b_switch}" | tail -1 | cut -d: -f1)"
  promote_gate="$(grep -n 'verify_release_image_ready' "${b_promote}" | cut -d: -f1)"
  promote_status="$(grep -n 'show_pre_promote_status' "${b_promote}" | tail -1 | cut -d: -f1)"

  ((sync_pull < sync_remote && sync_remote < sync_local)) \
    || fail_assertion "发布同步顺序不是先缓存、再 B、最后 A"
  ((recovery_image < stop_primary)) \
    || fail_assertion "A 恢复镜像校准必须早于停止旧服务"
  ((enable_gate < enable_promote)) \
    || fail_assertion "B enable 镜像门禁必须早于 promote"
  ((promote_gate < promote_status)) \
    || fail_assertion "promote 镜像门禁必须早于提升状态检查"
}

run_test "同标签更新解析当前运行 digest" test_resolves_new_digest_from_reused_tag
run_test "拒绝缺失或仓库不匹配的 RepoDigest" test_rejects_missing_and_wrong_repository_digest
run_test "B 只更新发布镜像与状态" test_b_updates_only_release_image
run_test "B 运行环境生成去除重复覆盖键" test_prepare_runtime_env_removes_duplicate_override_keys
run_test "B 非 standby 或复制不健康时拒绝写入" test_b_rejects_non_standby_and_unhealthy_replication
run_test "B enable 拒绝记录漂移和缓存缺失" test_b_enable_rejects_drift_and_missing_cache
run_test "B enable 接受已同步 digest 且 dry-run 无写入" test_b_enable_accepts_synced_digest_without_writes
run_test "A 同标签新 digest dry-run 报告漂移且无写入" test_a_sync_dry_run_detects_new_digest_without_writes
run_test "A 实际同步按 B 后 A 顺序收敛同标签新 digest" test_a_sync_updates_b_then_a_for_reused_tag
run_test "A/B SSH失败时在写入前停止" test_a_sync_stops_on_ssh_failure_before_writes
run_test "A 应用不健康时在远端访问前停止" test_a_sync_stops_on_unhealthy_app_before_remote_access
run_test "A 状态接口报告版本漂移" test_a_status_reports_drift
run_test "B 接管后 A dry-run 使用 B 活动 digest" test_a_recovery_uses_b_active_digest_in_dry_run
run_test "A/B 机器状态字段契约保持稳定" test_machine_status_field_contracts
run_test "缓存、配置与提升顺序保持稳定" test_destructive_order_contracts

printf '测试完成：通过=%d，失败=%d\n' "${PASS_COUNT}" "${FAIL_COUNT}"
[ "${FAIL_COUNT}" -eq 0 ]
