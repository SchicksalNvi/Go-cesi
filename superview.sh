#!/bin/bash
set -euo pipefail

# Superview 运维脚本
# 用法: ./superview.sh [start|stop|restart|status|run]

readonly APP_NAME="superview"
readonly PID_FILE="pids/backend.pid"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

get_pid() {
    [ -f "$PID_FILE" ] && cat "$PID_FILE" || true
}

is_running() {
    local pid
    pid=$(get_pid)
    [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null && \
        [ "$(readlink -f "/proc/$pid/exe" 2>/dev/null)" = "$(readlink -f "$(pwd)/$APP_NAME")" ]
}

check_binary() {
    if [ ! -f "$APP_NAME" ]; then
        log_error "Binary not found. Run 'make' first."
        return 1
    fi
}

start() {
    if is_running; then
        log_warn "Already running (PID: $(get_pid))"
        return 0
    fi
    check_binary
    mkdir -p pids logs data

    log_info "Starting $APP_NAME..."
    nohup "./$APP_NAME" > logs/app.log 2>&1 &
    echo $! > "$PID_FILE"

    sleep 1
    if is_running; then
        log_info "Started (PID: $(get_pid))"
        log_info "Access: http://localhost:8081"
    else
        log_error "Failed to start. Check logs/app.log"
        return 1
    fi
}

stop() {
    if ! is_running; then
        log_warn "Not running"
        rm -f "$PID_FILE"
        return 0
    fi

    local pid
    pid=$(get_pid)
    log_info "Stopping (PID: $pid)..."
    kill "$pid"

    for _ in {1..10}; do
        kill -0 "$pid" 2>/dev/null || { rm -f "$PID_FILE"; log_info "Stopped"; return 0; }
        sleep 1
    done

    kill -9 "$pid" 2>/dev/null || true
    rm -f "$PID_FILE"
    log_info "Force stopped"
}

restart() {
    stop
    sleep 1
    start
}

status() {
    if is_running; then
        log_info "Running (PID: $(get_pid))"
    else
        log_info "Not running"
    fi
}

run() {
    check_binary
    mkdir -p pids logs data
    log_info "Running in foreground (Ctrl+C to stop)..."
    exec "./$APP_NAME"
}

show_help() {
    cat << EOF
Superview 运维脚本

用法: $0 <command>

命令:
  start     后台启动
  stop      停止
  restart   重启
  status    查看状态
  run       前台运行

构建请使用 Makefile:
  make              构建前后端
  make release      打包发布
  make help         查看构建帮助
EOF
}

main() {
    cd "$(dirname "$0")"

    case "${1:-help}" in
        start)          start ;;
        stop)           stop ;;
        restart)        restart ;;
        status)         status ;;
        run)            run ;;
        help|-h|--help) show_help ;;
        *)
            log_error "Unknown command: $1"
            show_help
            exit 1
            ;;
    esac
}

main "$@"
