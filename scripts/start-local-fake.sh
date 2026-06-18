#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

# fake provider 用于前端交互和 Phase 9 smoke test，不读取真实 Codex 凭据。
exec "${SCRIPT_DIR}/start-local.sh" --config "${REPO_ROOT}/config/app.fake.json"
