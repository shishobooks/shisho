# Dev Servers Across Worktrees

Read this before adding or changing a `mise start*` or `mise docs` task, or anything that picks a dev port or names the session cookie.

Dev servers start through `cmd/dev` (`internal/devtool`) so several worktrees can run them at once. Its comments explain how ports are claimed and handed on.

- **Ports.** A new dev server reserves its port through `devtool`, never a hardcoded number, and passes it to anything that needs it through the launcher's environment. A fixed port collides with another worktree or another project on the machine. E2E suites pick their own free ports (`e2e/AGENTS.md`).
- **Cookies.** Browsers scope cookies by host, not port, so every worktree on `localhost` shares one cookie jar. The launcher namespaces the session cookie per linked worktree; a new dev cookie tied to one server's state would need the same treatment, or signing in to one worktree signs you out of another.
