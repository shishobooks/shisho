# Control share links from admin settings, not server config

Every other feature switch in Shisho is a config field with an env var (`demo_mode`, `library_monitor_enabled`). Share links are the first feature switched on from the database through the admin UI, with no config field and no env override. Share links expose a book to anyone holding a URL, so the switch belongs next to the disclaimer that the server must be reachable by the recipient and next to the companion policy (whether links must expire). An admin reads and decides those in one place; a YAML file cannot carry the warning or the policy. The default is off, so a fresh deployment shares nothing until an admin opts in.

## Considered options

- Config field plus env var, like every other switch. Rejected because the switch is not a deployment fact but a policy choice, and it comes with settings that only make sense alongside it.
- Both: a config kill switch plus the admin toggle. Rejected as two places to look for one answer. Demo Mode already covers the hosted case: the public share routes are not registered in Demo Mode, and settings writes are rejected there.

## Consequences

The sharing settings live in `app_settings` under one key, following the review criteria precedent. Reads are allowed with either the share permission or `config:read`, since anyone creating a link needs the policy to render the form and admins need it to render the page; writes require `config:write`. Turning sharing off stops every link from resolving but deletes nothing, so turning it back on restores them.
