#!/usr/bin/env bash

set -Eeuo pipefail

export PYTHONPATH=/opt/sub2api-ha-agent
exec /usr/bin/python3 -m sub2api_ha.cli verify-action "$@"
