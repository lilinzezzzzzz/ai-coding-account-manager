#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

APP_NAME="AI Coding Account Manager"
RUN_DIR="${REPO_ROOT}/.run"
LOG_DIR="${REPO_ROOT}/logs"
PID_FILE="${RUN_DIR}/server.pid"
BIN_FILE="${RUN_DIR}/ai-coding-account-manager"
LOG_FILE="${LOG_DIR}/server.log"
DEFAULT_CONFIG_FILE="${REPO_ROOT}/config/app.json"
FAKE_CONFIG_FILE="${REPO_ROOT}/config/app.fake.json"
DEFAULT_BIND_ADDR="127.0.0.1:43127"
REMOTE_UPDATE_NOTICE=""

if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  COLOR_RESET=$'\033[0m'
  COLOR_DIM=$'\033[2m'
  COLOR_BLUE=$'\033[34m'
  COLOR_GREEN=$'\033[32m'
  COLOR_YELLOW=$'\033[33m'
  COLOR_RED=$'\033[31m'
else
  COLOR_RESET=""
  COLOR_DIM=""
  COLOR_BLUE=""
  COLOR_GREEN=""
  COLOR_YELLOW=""
  COLOR_RED=""
fi

log_line() {
  local color="$1"
  local message="$2"
  printf '%s%s%s\n' "${color}" "${message}" "${COLOR_RESET}"
}

log_step() { log_line "${COLOR_BLUE}" "$1"; }
log_success() { log_line "${COLOR_GREEN}" "$1"; }
log_warn() { log_line "${COLOR_YELLOW}" "$1"; }
log_error() { log_line "${COLOR_RED}" "$1"; }

log_kv() {
  printf '  %s%s:%s %s\n' "${COLOR_DIM}" "$1" "${COLOR_RESET}" "$2"
}

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
  logs       Show background server logs from logs/server.log.
USAGE
}

ensure_runtime_dirs() {
  mkdir -p "${RUN_DIR}" "${LOG_DIR}" "${REPO_ROOT}/config"
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
  log_step "Building ${APP_NAME}"
  log_kv "output" "${BIN_FILE}"
  go build -trimpath -o "${BIN_FILE}" ./cmd/ai-coding-account-manager
}

config_bind_addr() {
  local config_file="$1"
  local bind_addr

  if [[ -f "${config_file}" ]]; then
    if command -v jq >/dev/null 2>&1; then
      bind_addr="$(jq -r '.bindAddr // empty' "${config_file}" 2>/dev/null || true)"
      if [[ -n "${bind_addr}" ]]; then
        printf '%s\n' "${bind_addr}"
        return 0
      fi
    elif command -v python3 >/dev/null 2>&1; then
      bind_addr="$(
        python3 -c '
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    bind_addr = json.load(f).get("bindAddr") or ""
print(bind_addr)
' "${config_file}" 2>/dev/null || true
      )"
      if [[ -n "${bind_addr}" ]]; then
        printf '%s\n' "${bind_addr}"
        return 0
      fi
    fi

    bind_addr="$(sed -nE 's/^[[:space:]]*"bindAddr"[[:space:]]*:[[:space:]]*"([^"]+)".*$/\1/p' "${config_file}" | sed -n '1p')"
    if [[ -n "${bind_addr}" ]]; then
      printf '%s\n' "${bind_addr}"
      return 0
    fi
  fi

  printf '%s\n' "${DEFAULT_BIND_ADDR}"
}

service_url() {
  local config_file="$1"
  local bind_addr
  local host
  local port

  bind_addr="$(config_bind_addr "${config_file}")"
  host="${bind_addr%:*}"
  port="${bind_addr##*:}"
  if [[ "${host}" == "0.0.0.0" ]]; then
    host="127.0.0.1"
  fi
  printf 'http://%s:%s/\n' "${host}" "${port}"
}

check_remote_updates() {
  local upstream
  local remote
  local behind_count

  REMOTE_UPDATE_NOTICE=""
  log_step "Checking remote updates"
  if ! git -C "${REPO_ROOT}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    log_warn "  status: skipped (not a git repository)"
    return 0
  fi
  if ! upstream="$(git -C "${REPO_ROOT}" rev-parse --abbrev-ref --symbolic-full-name '@{upstream}' 2>/dev/null)"; then
    log_warn "  status: skipped (no upstream configured)"
    return 0
  fi

  remote="${upstream%%/*}"
  if [[ -z "${remote}" || "${remote}" == "${upstream}" ]]; then
    log_warn "  status: skipped (invalid upstream: ${upstream})"
    return 0
  fi

  log_kv "upstream" "${upstream}"
  if ! git -C "${REPO_ROOT}" fetch --quiet "${remote}" 2>/dev/null; then
    set_remote_check_failed_notice "${upstream}"
    return 0
  fi
  if ! behind_count="$(git -C "${REPO_ROOT}" rev-list --count "HEAD..${upstream}" 2>/dev/null)"; then
    set_remote_check_failed_notice "${upstream}"
    return 0
  fi

  if [[ "${behind_count}" =~ ^[0-9]+$ && "${behind_count}" -gt 0 ]]; then
    log_warn "  status: remote has ${behind_count} new commit(s)"
    REMOTE_UPDATE_NOTICE="远程 ${upstream} 有 ${behind_count} 个新提交，服务已使用本地代码启动。可执行：git pull --ff-only"
    return 0
  fi

  log_success "  status: current branch is up to date"
}

set_remote_check_failed_notice() {
  local upstream="$1"
  log_warn "  status: remote check failed"
  REMOTE_UPDATE_NOTICE="未能确认远程更新（${upstream}），服务已使用本地代码启动。"
}

show_remote_update_notice() {
  if [[ -z "${REMOTE_UPDATE_NOTICE}" ]]; then
    return 0
  fi
  echo
  log_warn "Remote update notice:"
  log_kv "notice" "${REMOTE_UPDATE_NOTICE}"
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

# Find an orphaned process listening on the given port.
# Returns 0 with the PID on stdout if found, 1 otherwise.
find_orphan_pid() {
  local port="$1"
  local pid

  # Use ss to find the PID listening on this port
  pid="$(ss -tlnp "sport = :${port}" 2>/dev/null | sed -n 's/.*pid=\([0-9]*\).*/\1/p' | head -1)"

  if [[ -z "${pid}" ]]; then
    return 1
  fi

  # Check if the process is our binary
  local cmdline
  cmdline="$(cat /proc/${pid}/cmdline 2>/dev/null | tr '\0' ' ' || true)"
  if [[ "${cmdline}" == *"ai-coding-account-manager"* ]]; then
    printf '%s\n' "${pid}"
    return 0
  fi

  return 1
}

# Kill a process gracefully, falling back to SIGKILL.
kill_process() {
  local pid="$1"

  kill "${pid}" 2>/dev/null || true
  for _ in {1..50}; do
    if ! kill -0 "${pid}" 2>/dev/null; then
      return 0
    fi
    sleep 0.1
  done

  # Graceful shutdown timed out, force kill
  kill -9 "${pid}" 2>/dev/null || true
  sleep 0.2
}

# Resolve the port from a config file and kill any orphaned process on it.
# Returns 0 if an orphan was found and killed, 1 otherwise.
cleanup_orphan() {
  local config_file="$1"
  local bind_addr
  local port
  local orphan_pid

  bind_addr="$(config_bind_addr "${config_file}")"
  port="${bind_addr##*:}"

  if orphan_pid="$(find_orphan_pid "${port}")"; then
    log_warn "Found orphaned process on port ${port} (PID: ${orphan_pid})"
    log_kv "action" "Killing orphaned process..."
    kill_process "${orphan_pid}"
    rm -f "${PID_FILE}"
    log_success "  Orphaned process killed"
    return 0
  fi

  return 1
}

parse_run_args() {
  local default_config="$1"
  local allow_config="$2"
  local command_name="$3"
  shift 3

  CONFIG_FILE="${default_config}"
  FOREGROUND=0
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --config)
        if [[ "${allow_config}" -ne 1 ]]; then
          echo "Unknown argument for ${command_name}: $1" >&2
          usage >&2
          exit 2
        fi
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
        if [[ "${command_name}" == "start" || "${command_name}" == "restart" ]]; then
          echo "Unknown argument: $1" >&2
        else
          echo "Unknown argument for ${command_name}: $1" >&2
        fi
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

start_background() {
  local config_file="$1"
  local pid

  ensure_runtime_dirs
  if pid="$(running_pid)"; then
    log_warn "${APP_NAME} is already running"
    log_kv "pid" "${pid}"
    log_kv "url" "$(service_url "${config_file}")"
    log_kv "pid file" "${PID_FILE}"
    show_remote_update_notice
    exit 1
  fi

  # Clean up orphaned process on the target port (no PID file but process alive)
  cleanup_orphan "${config_file}" || true

  build_binary
  log_step "Starting ${APP_NAME}"
  log_kv "config file" "${config_file}"
  log_kv "log file" "${LOG_FILE}"
  log_kv "pid file" "${PID_FILE}"

  cd "${REPO_ROOT}"
  AI_CODING_ACCOUNT_MANAGER_LOG_FILE="${LOG_FILE}" "${BIN_FILE}" --config "${config_file}" >>"${LOG_FILE}" 2>&1 &
  pid="$!"
  echo "${pid}" >"${PID_FILE}"

  sleep 0.5
  if ! kill -0 "${pid}" 2>/dev/null; then
    rm -f "${PID_FILE}"
    log_error "${APP_NAME} failed to start"
    log_kv "log file" "${LOG_FILE}"
    tail -n 40 "${LOG_FILE}" 2>/dev/null || true
    exit 1
  fi

  log_success "Started"
  log_kv "pid" "${pid}"
  log_kv "url" "$(service_url "${config_file}")"
  log_kv "logs" "./scripts/local.sh logs --follow"
  log_kv "stop" "./scripts/local.sh stop"
  show_remote_update_notice
}

run_foreground() {
  local config_file="$1"

  build_binary
  log_step "Running ${APP_NAME}"
  log_kv "config file" "${config_file}"
  show_remote_update_notice
  cd "${REPO_ROOT}"
  exec "${BIN_FILE}" --config "${config_file}"
}

stop_background() {
  local pid

  if [[ ! -f "${PID_FILE}" ]]; then
    # No PID file — try to find and kill an orphaned process
    if cleanup_orphan "${DEFAULT_CONFIG_FILE}"; then
      return 0
    fi
    log_warn "${APP_NAME} is not running: pid file not found"
    log_kv "pid file" "${PID_FILE}"
    return 0
  fi

  pid="$(read_pid_file || true)"
  if ! [[ "${pid}" =~ ^[0-9]+$ ]]; then
    log_warn "Removing invalid pid file: ${PID_FILE}"
    rm -f "${PID_FILE}"
    return 0
  fi

  if ! kill -0 "${pid}" 2>/dev/null; then
    log_warn "Removing stale pid file: ${PID_FILE}"
    rm -f "${PID_FILE}"
    return 0
  fi

  log_step "Stopping ${APP_NAME}"
  log_kv "pid" "${pid}"
  kill "${pid}"

  for _ in {1..50}; do
    if ! kill -0 "${pid}" 2>/dev/null; then
      rm -f "${PID_FILE}"
      log_success "Stopped"
      return 0
    fi
    sleep 0.1
  done

  log_error "Process did not stop within 5s"
  log_kv "pid" "${pid}"
  log_kv "pid file" "${PID_FILE}"
  return 1
}

show_status() {
  local pid

  if pid="$(running_pid)"; then
    log_success "${APP_NAME} is running"
    log_kv "pid" "${pid}"
    log_kv "pid file" "${PID_FILE}"
    log_kv "log file" "${LOG_FILE}"
    return 0
  fi

  # Check for orphaned process (alive but no PID file)
  local bind_addr
  local port
  local orphan_pid
  bind_addr="$(config_bind_addr "${DEFAULT_CONFIG_FILE}")"
  port="${bind_addr##*:}"
  if orphan_pid="$(find_orphan_pid "${port}")"; then
    log_warn "${APP_NAME} is running (orphaned, no PID file)"
    log_kv "pid" "${orphan_pid}"
    log_kv "log file" "${LOG_FILE}"
    log_kv "to fix" "kill ${orphan_pid} && ./scripts/local.sh start"
    return 0
  fi

  log_warn "${APP_NAME} is not running"
  log_kv "pid file" "${PID_FILE}"
  return 3
}

show_logs() {
  ensure_runtime_dirs
  if [[ ! -f "${LOG_FILE}" ]]; then
    log_error "Log file not found: ${LOG_FILE}"
    exit 1
  fi

  if [[ "${FOLLOW_LOGS}" -eq 1 ]]; then
    exec tail -n 100 -F "${LOG_FILE}"
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
    parse_run_args "${DEFAULT_CONFIG_FILE}" 1 "start" "$@"
    check_remote_updates
    if [[ "${FOREGROUND}" -eq 1 ]]; then
      run_foreground "${CONFIG_FILE}"
    fi
    start_background "${CONFIG_FILE}"
    ;;
  fake)
    parse_run_args "${FAKE_CONFIG_FILE}" 0 "fake" "$@"
    if [[ "${FOREGROUND}" -eq 1 ]]; then
      run_foreground "${CONFIG_FILE}"
    fi
    start_background "${CONFIG_FILE}"
    ;;
  stop)
    stop_background
    ;;
  restart)
    parse_run_args "${DEFAULT_CONFIG_FILE}" 1 "restart" "$@"
    check_remote_updates
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
