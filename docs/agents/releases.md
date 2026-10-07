# Breaking Changes and Releases

Read this before writing a PR body for a breaking change, cutting a release, or changing anything under `scripts/` or the release workflow.

## Breaking change upgrade notes

A change is breaking when an operator has to do something before or after upgrading: a renamed or removed config key or env var, a changed default, a removed route or response field, a new startup validation that can refuse an existing config, or a changed on-disk layout. Root `AGENTS.md` requires two markers: `!` after the category in the title, and a `## BREAKING CHANGES` section in the PR body.

Write one bullet per change for an operator who is upgrading: what changed, what they must do, and what happens if they do not. Pull requests squash-merge with the PR body as the commit message, so these bullets end up in git history and `scripts/release.sh` copies them into the changelog under the commit's subject. There is no way to add notes at release time.

How the generator reads the section:

- List items (`-`, `*`, or `1.`, with their wrapped lines), plain paragraphs (each becomes a bullet), and fenced code blocks are copied.
- A `Closes #N` or `Fixes #N` line and trailers such as `Co-authored-by:` are dropped.
- The section ends at the next heading of the same level; a deeper heading inside it becomes a bold bullet.
- The heading alone marks the commit as breaking even when the title has no `!`, and `!` alone still lists the commit under Breaking Changes when the body section is missing.
- A literal `{{` in the notes is escaped before it reaches GoReleaser's template renderer.

## Releases

- `mise release 0.2.0` creates a release; `mise release 0.2.0 --dry-run` prints the changelog entry without changing anything.
- `scripts/release.sh`:
  1. Generates the changelog entry from commits since the last tag with `scripts/lib/changelog.sh`: a `### Breaking Changes` block first (commits marked with `!` or carrying a `## BREAKING CHANGES` body section, with their upgrade notes nested under the subject), then the category sections.
  2. Updates `CHANGELOG.md`, `package.json`, and `packages/plugin-sdk/package.json`.
  3. Creates a commit `[Release] v0.2.0`.
  4. Tags and pushes to trigger GitHub Actions.
- The release workflow runs `scripts/release-notes-header.sh` to build the GitHub release header from the install block plus that version's `### Breaking Changes` block in `CHANGELOG.md`, and passes it to GoReleaser with `--release-header-tmpl`. That header is the only Breaking Changes section on the release page; GoReleaser's own commit list below it groups by category only, because it reads subjects and would repeat the headline without the notes.
- `mise test:scripts` runs the shell script tests against a throwaway git repository. Run it after changing anything under `scripts/`.
