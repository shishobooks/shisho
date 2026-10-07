# Breaking Changes and Releases

Read this before writing a PR body for a breaking change, cutting a release, or changing anything under `scripts/` or the release workflow.

## Breaking change upgrade notes

A change is breaking when an operator has to do something before or after upgrading: a renamed or removed config key or env var, a changed default, a removed route or response field, a new startup validation that can refuse an existing config, or a changed on-disk layout. Root `AGENTS.md` requires two markers: `!` after the category in the title, and a `## BREAKING CHANGES` section in the PR body.

Write one bullet per change for an operator who is upgrading: what changed, what they must do, and what happens if they do not. Pull requests squash-merge with the PR body as the commit message, so these bullets end up in git history and `scripts/release.sh` copies them into the changelog under the commit's subject. There is no way to add notes at release time.

What the generator copies from the section is defined in `scripts/lib/changelog.sh` and pinned by `scripts/changelog_test.sh`. The section runs to the next heading of its level, and either marker alone lists the commit under Breaking Changes.

## Releases

- `mise release 0.2.0` cuts a release (`--dry-run` prints the changelog entry and changes nothing); `scripts/release.sh` is the procedure.
- The GitHub release header built by `scripts/release-notes-header.sh` is the only Breaking Changes section on the release page. GoReleaser's own commit list groups by category only, because it reads subjects and would repeat the headline without the notes.
- Run `mise test:scripts` after changing anything under `scripts/`.
