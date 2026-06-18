#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

APP_NAME="AI Coding Account Manager"
RUN_DIR="${REPO_ROOT}/.run"
PID_FILE="${RUN_DIR}/server.pid"
BIN_FILE="${RUN_DIR}/ai-coding-account-manager"
LOG_FILE="${RUN_DIR}/server.log"
DEFAULT_CONFIG_FILE="${REPO_ROOT}/config/app.json"
FAKE_CONFIG_FILE="${REPO_ROOT}/config/app.fake.json"

usage() {
  cat <<'USAGE'
Usage:
  ./scripts/local.sh start [--config FILE] [--foreground]
  ./scripts/local.sh fake [--foreground]
  ./scripts/local.sh stop
  ./scripts/local.sh restart [--config FILE]
  ./scripts/local.sh status
  ./scripts/local.sh logs [--follow|-f]

Commands:
  start      Build and start the local server in background.
  fake       Start with config/app.fake.json for UI smoke checks.
  stop       Stop the background server started by this script.
  restart    Stop then start the background server.
  status     Show background server status.
  logs       Show background server logs from .run/server.log.
USAGE
}

ensure_runtime_dirs() {
  mkdir -p "${RUN_DIR}" "${REPO_ROOT}/config"
}

resolve_path() {
  local value="$1"
  if [[ "${value}" == /* ]]; then
    printf '%s\n' "${value}"
    return
  fi
  printf '%s\n' "${REPO_ROOT}/${value}"
}

build_binary() {
  ensure_runtime_dirs
  cd "${REPO_ROOT}"
  echo "Building ${APP_NAME}"
  echo "  output: ${BIN_FILE}"
  go build -trimpath -o "${BIN_FILE}" ./cmd/ai-coding-account-manager
}

read_pid_file() {
  if [[ ! -f "${PID_FILE}" ]]; then
    return 1
  fi
  tr -d '[:space:]' <"${PID_FILE}"
}

running_pid() {
  local pid
  if ! pid="$(read_pid_file)"; then
    return 1
  fi
  if ! [[ "${pid}" =~ ^[0-9]+$ ]]; then
    rm -f "${PID_FILE}"
    return 1
  fi
  if ! kill -0 "${pid}" 2>/dev/null; then
    rm -f "${PID_FILE}"
    return 1
  fi
  printf '%s\n' "${pid}"
}

parse_config_args() {
  CONFIG_FILE="${DEFAULT_CONFIG_FILE}"
  FOREGROUND=0
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --config)
        if [[ -z "${2:-}" ]]; then
          echo "--config requires a file path" >&2
          exit 2
        fi
        CONFIG_FILE="$(resolve_path "$2")"
        shift 2
        ;;
      --foreground)
        FOREGROUND=1
        shift
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      *)
        echo "Unknown argument: $1" >&2
        usage >&2
        exit 2
        ;;
    esac
  done
}

parse_logs_args() {
  FOLLOW_LOGS=0
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --follow|-f)
        FOLLOW_LOGS=1
        shift
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      *)
        echo "Unknown argument: $1" >&2
        usage >&2
        exit 2
        ;;
    esac
  done
}

parse_fake_args() {
  FOREGROUND=0
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --foreground)
        FOREGROUND=1
        shift
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      *)
        echo "Unknown argument for fake: $1" >&2
        usage >&2
        exit 2
        ;;
    esac
  done
}

start_background() {
  local config_file="$1"
  local pid

  ensure_runtime_dirs
  if pid="$(running_pid)"; then
    echo "${APP_NAME} is already running"
    echo "  pid: ${pid}"
    echo "  pid file: ${PID_FILE}"
    exit 1
  fi

  build_binary
  echo "Starting ${APP_NAME}"
  echo "  config file: ${config_file}"
  echo "  log file: ${LOG_FILE}"
  echo "  pid file: ${PID_FILE}"

  cd "${REPO_ROOT}"
  "${BIN_FILE}" --config "${config_file}" >>"${LOG_FILE}" 2>&1 &
  pid="$!"
  echo "${pid}" >"${PID_FILE}"

  sleep 0.5
  if ! kill -0 "${pid}" 2>/dev/null; then
    rm -f "${PID_FILE}"
    echo "${APP_NAME} failed to start"
    echo "  log file: ${LOG_FILE}"
    tail -n 40 "${LOG_FILE}" 2>/dev/null || true
    exit 1
  fi

  echo "Started"
  echo "  pid: ${pid}"
  echo "  logs: ./scripts/local.sh logs --follow"
  echo "  stop: ./scripts/local.sh stop"
}

run_foreground() {
  local config_file="$1"

  build_binary
  echo "Running ${APP_NAME}"
  echo "  config file: ${config_file}"
  cd "${REPO_ROOT}"
  exec "${BIN_FILE}" --config "${config_file}"
}

stop_background() {
  local pid

  if [[ ! -f "${PID_FILE}" ]]; then
    echo "${APP_NAME} is not running: pid file not found"
    echo "  pid file: ${PID_FILE}"
    return 0
  fi

  pid="$(read_pid_file || true)"
  if ! [[ "${pid}" =~ ^[0-9]+$ ]]; then
    echo "Removing invalid pid file: ${PID_FILE}"
    rm -f "${PID_FILE}"
    return 0
  fi

  if ! kill -0 "${pid}" 2>/dev/null; then
    echo "Removing stale pid file: ${PID_FILE}"
    rm -f "${PID_FILE}"
    return 0
  fi

  echo "Stopping ${APP_NAME}"
  echo "  pid: ${pid}"
  kill "${pid}"

  for _ in {1..50}; do
    if ! kill -0 "${pid}" 2>/dev/null; then
      rm -f "${PID_FILE}"
      echo "Stopped"
      return 0
    fi
    sleep 0.1
  done

  echo "Process did not stop within 5s"
  echo "  pid: ${pid}"
  echo "  pid file: ${PID_FILE}"
  return 1
}

show_status() {
  local pid

  if pid="$(running_pid)"; then
    echo "${APP_NAME} is running"
    echo "  pid: ${pid}"
    echo "  pid file: ${PID_FILE}"
    echo "  log file: ${LOG_FILE}"
    return 0
  fi

  echo "${APP_NAME} is not running"
  echo "  pid file: ${PID_FILE}"
  return 3
}

show_logs() {
  ensure_runtime_dirs
  if [[ ! -f "${LOG_FILE}" ]]; then
    echo "Log file not found: ${LOG_FILE}"
    exit 1
  fi

  if [[ "${FOLLOW_LOGS}" -eq 1 ]]; then
    exec tail -n 100 -f "${LOG_FILE}"
  fi
  tail -n 100 "${LOG_FILE}"
}

if [[ $# -lt 1 ]]; then
  usage
  exit 2
fi

command="$1"
shift

case "${command}" in
  start)
    parse_config_args "$@"
    if [[ "${FOREGROUND}" -eq 1 ]]; then
      run_foreground "${CONFIG_FILE}"
    fi
    start_background "${CONFIG_FILE}"
    ;;
  fake)
    parse_fake_args "$@"
    CONFIG_FILE="${FAKE_CONFIG_FILE}"
    if [[ "${FOREGROUND}" -eq 1 ]]; then
      run_foreground "${CONFIG_FILE}"
    fi
    start_background "${CONFIG_FILE}"
    ;;
  stop)
    stop_background
    ;;
  restart)
    parse_config_args "$@"
    stop_background
    start_background "${CONFIG_FILE}"
    ;;
  status)
    show_status
    ;;
  logs)
    parse_logs_args "$@"
    show_logs
    ;;
  --help|-h|help)
    usage
    ;;
  *)
    echo "Unknown command: ${command}" >&2
    usage >&2
    exit 2
    ;;
esac
