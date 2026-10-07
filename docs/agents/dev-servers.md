# Dev Servers Across Worktrees

Read this before adding or changing a `mise start*` or `mise docs` task, or anything that picks a dev port or names the session cookie.

`mise start`, `start:air`, `start:api`, `start:web`, and `mise docs` run `cmd/dev` (`internal/devtool`), so several worktrees can run them at once.

- **Ports.** Each server takes the first free port at or above its usual one (API `3689`, Vite `5173`, docs `3000`) and prints the URL it got. Ports are claimed with `flock` on files in the main worktree's `tmp/ports/`, which every linked worktree shares, so two worktrees starting together can't pick the same port. A port is also skipped when anything else listens on it on `127.0.0.1`, `0.0.0.0`, `::1`, or `::`, including other projects; `::1` counts because browsers resolve `localhost` there first. A new task reserves its port through `devtool`, never a hardcoded number. E2E suites pick their own free ports (`e2e/AGENTS.md`).
- **Handing the API port on.** The launcher passes it to air as `SERVER_PORT` and to Vite as `API_PORT`. `start:web` (Vite only, proxying to the API this worktree started with `start:air`) reads it from `tmp/api.port`, which the API writes on startup only when the launcher sets `SHISHO_DEV_PORT_FILE`, so an E2E API in the same worktree can't overwrite it. The API removes the file on graceful shutdown and the launcher removes it when it exits, so a killed API can't leave one behind.
- **Session cookies.** Browsers scope cookies by host, not port, so in linked worktrees the launcher sets `SHISHO_COOKIE_NAMESPACE` (the directory name plus a path hash). That renames the session cookie to `shisho_session_<namespace>`, so signing in to one worktree doesn't sign you out of another. The main worktree keeps `shisho_session`.
