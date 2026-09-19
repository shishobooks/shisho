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

# =============================================================================
# User/Group Setup
# =============================================================================
# If running as root (default), set up proper user and drop privileges.
# If running with --user flag, skip this and run directly as that user.

if [ "$(id -u)" = "0" ]; then
    # Running as root - set up user with custom PUID/PGID if provided
    PUID=${PUID:-1000}
    PGID=${PGID:-1000}

    # Update shisho user/group if PUID or PGID differs from default
    if [ "$PGID" != "1000" ] || [ "$PUID" != "1000" ]; then
        # Must delete user first (before group), since user is a member of the group
        deluser shisho 2>/dev/null || true

        # Determine which group to use
        if [ "$PGID" = "1000" ]; then
            # Use existing shisho group
            TARGET_GROUP="shisho"
        else
            # Check if a group with the desired GID already exists
            EXISTING_GROUP=$(getent group "$PGID" | cut -d: -f1)
            if [ -n "$EXISTING_GROUP" ]; then
                # Reuse the existing group (e.g., "users" for GID 100)
                TARGET_GROUP="$EXISTING_GROUP"
                delgroup shisho 2>/dev/null || true
            else
                # Create new shisho group with desired GID
                delgroup shisho 2>/dev/null || true
                addgroup -g "$PGID" shisho
                TARGET_GROUP="shisho"
            fi
        fi

        adduser -u "$PUID" -G "$TARGET_GROUP" -s /bin/sh -D shisho
    fi

    # Fix ownership of config directory (use numeric IDs in case group name differs)
    chown -R "$PUID:$PGID" /config

    log_info "Starting as shisho (UID=$PUID, GID=$PGID)"
    exec su-exec shisho /app/shisho "$@"
fi

log_info "Running as $(id -un) (UID=$(id -u), GID=$(id -g))"
exec /app/shisho "$@"
