#!/bin/sh
set -e

# =============================================================================
# Logging Helper
# =============================================================================
# Output logs in JSON format to match the Go backend's log format

log() {
    level="$1"
    msg="$2"
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    if [ "$LOG_FORMAT" = "json" ]; then
        printf '{"level":"%s","time":"%s","msg":"%s"}\n' "$level" "$timestamp" "$msg"
    else
        printf "[%s] %s: %s\n" "$timestamp" "$level" "$msg"
    fi
}

log_info() { log "info" "$1"; }
log_error() { log "error" "$1"; }

# =============================================================================
# User/Group Setup
# =============================================================================
# If running as root (default), set up proper user and drop privileges.
# If running with --user flag, skip this and run directly as that user.

if [ "$(id -u)" = "0" ]; then
    # Running as root - set up user with custom PUID/PGID if provided
    PUID=${PUID:-1000}
    PGID=${PGID:-1000}

    # Update shisho group if PGID differs from default
    if [ "$PGID" != "1000" ]; then
        delgroup shisho 2>/dev/null || true
        addgroup -g "$PGID" shisho
        # Re-add user to new group
        deluser shisho 2>/dev/null || true
        adduser -u "$PUID" -G shisho -s /bin/sh -D shisho
    elif [ "$PUID" != "1000" ]; then
        # Only PUID differs
        deluser shisho 2>/dev/null || true
        adduser -u "$PUID" -G shisho -s /bin/sh -D shisho
    fi

    # Fix ownership of config directory
    chown -R shisho:shisho /config

    log_info "Starting as shisho (UID=$PUID, GID=$PGID)"

    # Re-exec this script as the shisho user
    exec su-exec shisho "$0" "$@"
fi

# =============================================================================
# Start Services (running as non-root user)
# =============================================================================

log_info "Running as $(id -un) (UID=$(id -u), GID=$(id -g))"

# Signal handler to forward signals to both processes
SHISHO_PID=""
CADDY_PID=""

cleanup() {
    log_info "Received shutdown signal, forwarding to child processes"

    # Forward signal to both processes
    if [ -n "$CADDY_PID" ] && kill -0 $CADDY_PID 2>/dev/null; then
        log_info "Stopping Caddy (PID $CADDY_PID)"
        kill -TERM $CADDY_PID 2>/dev/null
    fi

    if [ -n "$SHISHO_PID" ] && kill -0 $SHISHO_PID 2>/dev/null; then
        log_info "Stopping backend (PID $SHISHO_PID)"
        kill -TERM $SHISHO_PID 2>/dev/null
    fi

    # Wait for processes to terminate
    if [ -n "$CADDY_PID" ]; then
        wait $CADDY_PID 2>/dev/null
    fi
    if [ -n "$SHISHO_PID" ]; then
        wait $SHISHO_PID 2>/dev/null
    fi

    log_info "Shutdown complete"
    exit 0
}

# Trap SIGTERM and SIGINT
trap cleanup TERM INT

# Start the Go backend in the background
/app/shisho &
SHISHO_PID=$!

# Wait for backend to be ready
log_info "Waiting for backend to start"
for i in $(seq 1 30); do
    if wget -q --spider http://localhost:3689/health 2>/dev/null; then
        log_info "Backend is ready"
        break
    fi
    if ! kill -0 $SHISHO_PID 2>/dev/null; then
        log_error "Backend process died unexpectedly"
        exit 1
    fi
    sleep 1
done

# Start Caddy in the background (so we can manage both processes)
caddy run --config /etc/caddy/Caddyfile --adapter caddyfile &
CADDY_PID=$!

log_info "Started Caddy (PID $CADDY_PID) and backend (PID $SHISHO_PID)"

# Wait for either process to exit
# If one exits unexpectedly, the other should be stopped too
while true; do
    if ! kill -0 $SHISHO_PID 2>/dev/null; then
        log_error "Backend process exited"
        kill -TERM $CADDY_PID 2>/dev/null
        wait $CADDY_PID 2>/dev/null
        exit 1
    fi
    if ! kill -0 $CADDY_PID 2>/dev/null; then
        log_error "Caddy process exited"
        kill -TERM $SHISHO_PID 2>/dev/null
        wait $SHISHO_PID 2>/dev/null
        exit 1
    fi
    sleep 1
done
