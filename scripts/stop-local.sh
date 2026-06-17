#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

RUN_DIR="${AI_CODING_ACCOUNT_MANAGER_RUN_DIR:-${REPO_ROOT}/.run}"
PID_FILE="${AI_CODING_ACCOUNT_MANAGER_PID_FILE:-${RUN_DIR}/server.pid}"

if [[ ! -f "${PID_FILE}" ]]; then
  echo "AI Coding Account Manager is not running: pid file not found"
  echo "  pid file: ${PID_FILE}"
  exit 0
fi

service_pid="$(tr -d '[:space:]' <"${PID_FILE}")"
if ! [[ "${service_pid}" =~ ^[0-9]+$ ]]; then
  echo "Removing invalid pid file: ${PID_FILE}"
  rm -f "${PID_FILE}"
  exit 0
fi

if ! kill -0 "${service_pid}" 2>/dev/null; then
  echo "Removing stale pid file: ${PID_FILE}"
  rm -f "${PID_FILE}"
  exit 0
fi

echo "Stopping AI Coding Account Manager"
echo "  pid: ${service_pid}"
kill "${service_pid}"

for _ in {1..50}; do
  if ! kill -0 "${service_pid}" 2>/dev/null; then
    rm -f "${PID_FILE}"
    echo "Stopped"
    exit 0
  fi
  sleep 0.1
done

echo "Process did not stop within 5s"
echo "  pid: ${service_pid}"
echo "  pid file: ${PID_FILE}"
exit 1
