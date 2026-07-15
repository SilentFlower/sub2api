#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
AUTOMATION_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
TARGET="${AUTOMATION_ROOT}/bin/sync-release-if-needed.sh"
TEMPORARY="$(mktemp -d)"
trap 'rm -rf -- "${TEMPORARY}"' EXIT

STATE_FILE="${TEMPORARY}/state"
CALLS_FILE="${TEMPORARY}/calls"

cat >"${TEMPORARY}/switch-mode.sh" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
case "$*" in
  "status --machine") cat "${STATE_FILE}" ;;
  "sync-release --dry-run") printf 'dry-run\n' >>"${CALLS_FILE}" ;;
  "sync-release")
    printf 'sync\n' >>"${CALLS_FILE}"
    sed -i 's/^image_sync=.*/image_sync=ok/' "${STATE_FILE}"
    ;;
  *) exit 2 ;;
esac
EOF

cat >"${TEMPORARY}/docker" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
[ "$1" = "inspect" ]
printf '2020-01-01T00:00:00Z\n'
EOF

chmod 0755 "${TEMPORARY}/switch-mode.sh" "${TEMPORARY}/docker"
export STATE_FILE CALLS_FILE

write_state() {
  local sync="$1"
  cat >"${STATE_FILE}" <<EOF
mode=legacy-active
app_container=running
image_sync=${sync}
EOF
}

run_target() {
  SWITCH_MODE_COMMAND="${TEMPORARY}/switch-mode.sh" \
    DOCKER_COMMAND="${TEMPORARY}/docker" \
    LOCK_FILE="${TEMPORARY}/release-sync.lock" \
    STABLE_SECONDS="${1}" \
    "${TARGET}"
}

write_state ok
run_target 0
[ ! -e "${CALLS_FILE}" ]

write_state drift
run_target 999999999
[ ! -e "${CALLS_FILE}" ]

run_target 0
[ "$(cat "${CALLS_FILE}")" = $'dry-run\nsync' ]
grep -qx 'image_sync=ok' "${STATE_FILE}"

printf 'release sync tests passed\n'
