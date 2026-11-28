# Breaking Changes and Releases

## Breaking change upgrade notes

Root `AGENTS.md` defines a breaking change and its two markers. Write one bullet per change for an operator or user who is upgrading: what changed, what they must do, and what happens if they do not.

Pull requests squash-merge with the PR body as the commit message, so these bullets end up in git history and `scripts/release.sh` copies them into the changelog under the commit's subject. There is no way to add notes at release time. What the generator copies from the section is defined in `scripts/lib/changelog.sh` and pinned by `scripts/changelog_test.sh`; read those rather than guessing at the format.

## Releases

`mise release <version>` cuts a release (`--dry-run` prints the changelog entry and changes nothing); `scripts/release.sh` is the procedure. The comments in `.goreleaser.yaml` explain why breaking changes appear only in the release header.

## TestFlight builds of the Shisho app

`.github/workflows/testflight.yml` uploads an iOS build to App Store Connect when a push to master finds the app changed since the last successful run, as `scripts/testflight-changes.sh` decides, and on a manual run on master from the Actions tab. App Store Connect gives each processed build to the internal testing group, the maintainer and their partner, through the group's automatic distribution; internal testers need no Beta App Review. TestFlight's copy is the everyday Shisho on their phones. How the archive is signed, and why the API key in the repository secrets is an Admin key, is in the comments on `ios:archive` and `ios:upload` in `mise.toml`.

- **A failed upload goes out with the next push to master, whatever it changes.** To ship it sooner, start a manual run; don't rerun the failed one once it got as far as uploading, since a rerun keeps its run number, which is the build number, and App Store Connect refuses a number it already has.
- **A change only to how the build is made, such as the tasks in `mise.toml`, uploads nothing by itself.** Start a manual run to ship it.
- **To check signing without uploading**, run `mise ios:archive`, then `mise ios:upload --export-only` with the API key in the environment it names. `codesign -d --entitlements -` on the app in the `.ipa` it writes should list CarPlay's. An export from an archive that lost them still succeeds, so look. `xcrun altool --validate-app` with the same key then runs App Store Connect's checks on the `.ipa` without using up a build number.
