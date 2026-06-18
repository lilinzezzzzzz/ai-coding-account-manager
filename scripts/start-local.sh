#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_ROOT}"

RUN_DIR="${REPO_ROOT}/.run"
PID_FILE="${RUN_DIR}/server.pid"
BIN_FILE="${RUN_DIR}/ai-coding-account-manager"
CONFIG_FILE="${REPO_ROOT}/config/app.json"

if [[ "${1:-}" == "--config" ]] && [[ -n "${2:-}" ]]; then
  CONFIG_FILE="${2}"
fi

mkdir -p "${RUN_DIR}" "${REPO_ROOT}/config"

if [[ -f "${PID_FILE}" ]]; then
  existing_pid="$(tr -d '[:space:]' <"${PID_FILE}")"
  if [[ "${existing_pid}" =~ ^[0-9]+$ ]] && kill -0 "${existing_pid}" 2>/dev/null; then
    echo "AI Coding Account Manager is already running"
    echo "  pid: ${existing_pid}"
    echo "  pid file: ${PID_FILE}"
    echo "  stop: ./scripts/stop-local.sh"
    exit 1
  fi
  echo "Removing stale pid file: ${PID_FILE}"
  rm -f "${PID_FILE}"
fi

echo "Starting AI Coding Account Manager"
echo "  config file: ${CONFIG_FILE}"
echo "  run dir: ${RUN_DIR}"
echo "  frontend: served by Go server from frontend/static"
echo "  pid file: ${PID_FILE}"

go build -trimpath -o "${BIN_FILE}" ./cmd/ai-coding-account-manager

"${BIN_FILE}" "$@" &
service_pid="$!"
echo "${service_pid}" >"${PID_FILE}"
echo "  pid: ${service_pid}"

cleanup() {
  status="$?"
  if [[ -f "${PID_FILE}" ]] && [[ "$(tr -d '[:space:]' <"${PID_FILE}")" == "${service_pid}" ]]; then
    rm -f "${PID_FILE}"
  fi
  exit "${status}"
}

stop_child() {
  if kill -0 "${service_pid}" 2>/dev/null; then
    kill "${service_pid}" 2>/dev/null || true
  fi
}

trap cleanup EXIT
trap 'stop_child; exit 130' INT
trap 'stop_child; exit 143' TERM

set +e
wait "${service_pid}"
status="$?"
set -e
exit "${status}"
