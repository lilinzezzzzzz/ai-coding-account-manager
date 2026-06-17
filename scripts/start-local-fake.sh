#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

# fake provider 用于前端交互和 Phase 9 smoke test，不读取真实 Codex 凭据。
export AI_CODING_ACCOUNT_MANAGER_PROVIDER_MODE=fake

exec "${SCRIPT_DIR}/start-local.sh"
