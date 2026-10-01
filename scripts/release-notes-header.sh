#!/usr/bin/env bash
set -euo pipefail

# Prints the GitHub release notes header for a version: the install block that
# used to live in .goreleaser.yaml's release.header, followed by the
# "### Breaking Changes" block from that version's CHANGELOG.md entry when
# there is one. The release workflow writes the output to a file and passes it
# to goreleaser with --release-header-tmpl, so {{ .Tag }} style placeholders are
# still rendered by goreleaser.
#
# Usage: ./scripts/release-notes-header.sh <version> [changelog-file]
# Example: ./scripts/release-notes-header.sh 0.2.0

VERSION="${1:-}"
CHANGELOG_FILE="${2:-CHANGELOG.md}"

if [[ -z "$VERSION" ]]; then
    echo "Usage: $0 <version> [changelog-file]" >&2
    exit 1
fi
VERSION="${VERSION#v}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/changelog.sh
source "$SCRIPT_DIR/lib/changelog.sh"

cat <<'EOF'
## Shisho {{ .Tag }}

Docker image: `ghcr.io/shishobooks/shisho:{{ trimprefix .Tag "v" }}`

```bash
docker pull ghcr.io/shishobooks/shisho:{{ trimprefix .Tag "v" }}
```
EOF

if ! grep -q "^## \[$VERSION\]" "$CHANGELOG_FILE"; then
    # A tag pushed by hand has no entry; the release still gets the install
    # block, and the Actions log carries a warning.
    echo "::warning::$CHANGELOG_FILE has no entry for $VERSION; the release header has no Breaking Changes block" >&2
fi

BREAKING=$(breaking_changes_from_changelog "$VERSION" "$CHANGELOG_FILE")
if [[ -n "$BREAKING" ]]; then
    # The file is a Go template, so a literal "{{" copied from a PR body would
    # break rendering (or expand an action). Escape it.
    BREAKING=$(printf '%s\n' "$BREAKING" | sed 's/{{/{{"{{"}}/g')
    printf '\n### Breaking Changes\n\nRead these before upgrading.\n\n%s\n' "$BREAKING"
fi
