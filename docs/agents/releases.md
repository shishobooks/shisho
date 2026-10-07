# Breaking Changes and Releases

## Breaking change upgrade notes

Root `AGENTS.md` defines a breaking change and its two markers. Write one bullet per change for an operator who is upgrading: what changed, what they must do, and what happens if they do not.

Pull requests squash-merge with the PR body as the commit message, so these bullets end up in git history and `scripts/release.sh` copies them into the changelog under the commit's subject. There is no way to add notes at release time. What the generator copies from the section is defined in `scripts/lib/changelog.sh` and pinned by `scripts/changelog_test.sh`; read those rather than guessing at the format.

## Releases

`mise release <version>` cuts a release (`--dry-run` prints the changelog entry and changes nothing); `scripts/release.sh` is the procedure. The comments in `.goreleaser.yaml` explain why breaking changes appear only in the release header.
