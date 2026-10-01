#!/usr/bin/env bash
# Tests for scripts/lib/changelog.sh against a throwaway git repository.
# Run with: ./scripts/changelog_test.sh (or `mise test:scripts`).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/changelog.sh
source "$SCRIPT_DIR/lib/changelog.sh"

FAILURES=0
fail() {
    echo "FAIL: $1" >&2
    FAILURES=$((FAILURES + 1))
}
assert_contains() {
    local haystack="$1" needle="$2" label="$3"
    if [[ "$haystack" != *"$needle"* ]]; then
        fail "$label: expected to find:"$'\n'"$needle"$'\n'"in:"$'\n'"$haystack"
    fi
}
assert_not_contains() {
    local haystack="$1" needle="$2" label="$3"
    if [[ "$haystack" == *"$needle"* ]]; then
        fail "$label: did not expect to find:"$'\n'"$needle"$'\n'"in:"$'\n'"$haystack"
    fi
}

REPO=$(mktemp -d)
trap 'rm -rf "$REPO"' EXIT
cd "$REPO"
git init -q -b master .
git config user.email test@example.com
git config user.name test
git config commit.gpgsign false

commit() {
    # commit <subject> [body]
    echo "$RANDOM" >> file.txt
    git add file.txt
    if [[ $# -gt 1 ]]; then
        git commit -q -m "$1" -m "$2"
    else
        git commit -q -m "$1"
    fi
}

commit "[Release] v0.0.1"
git tag v0.0.1

commit "[Feature] Add dark mode (#1)" "## What changed

A fenced example of the heading must not start a section:

\`\`\`markdown
## BREAKING CHANGES
- not a real note
\`\`\`

Closes #1"
commit "[Fix]! Rename the test mode key (#2)" "## What changed

Nothing to see here.

## BREAKING CHANGES

Operators, read this first.

- **Test routes need their own key.** Hosts that set \`ENVIRONMENT=test\`
no longer get the test routes. Set \`SHISHO_TEST_MODE\` instead.
-	Tab after the dash.
1. Numbered items count too.
2) So does this style.
- Example config:

  \`\`\`yaml
  # a comment inside a fence is not a heading
  test_mode: true
  \`\`\`

A trailing paragraph becomes its own bullet.

## Test evidence

- This bullet must not leak into the breaking section."
commit "[Fix] Fix a typo (#3)" "### Breaking Change

Lowercase heading, singular noun and level three still count.
A paragraph note is kept, like the one on #595.

Fixes #30
Closes #3
Co-authored-by: Someone <someone@example.com>"
commit "[Backend]! Drop the legacy endpoint (#4)"
commit "Unprefixed commit (#5)"
commit "[Fix]Missing space after the category (#6)"
commit "[Docs] Heading only, no title marker (#7)" "## BREAKING CHANGES

### Config

- Nested under a deeper heading.

Closes #7"
commit "[Fix] Template syntax in a note (#8)" "## BREAKING CHANGES

- Set \`{{ .Env.HOME }}\` style values by hand now."

SECTION=$(generate_changelog_section 0.0.2 v0.0.1..HEAD)

# Section order and category lists still work, with the "!" stripped.
assert_contains "$SECTION" "## [0.0.2] - $(date +%Y-%m-%d)" "version heading"
# Lists are newest first, like git log, matching the existing changelog.
assert_contains "$SECTION" $'### Features\n- Drop the legacy endpoint (#4)\n- Add dark mode (#1)' "features list"
assert_contains "$SECTION" $'### Bug Fixes\n- Template syntax in a note (#8)\n- Missing space after the category (#6)\n- Fix a typo (#3)\n- Rename the test mode key (#2)' "bug fixes list"
assert_contains "$SECTION" $'### Documentation\n- Heading only, no title marker (#7)' "docs list"
assert_contains "$SECTION" $'### Other\n- Unprefixed commit (#5)' "other list"
assert_not_contains "$SECTION" "]! " "marker leaks into a category list"

# Breaking Changes comes first and lists every marked commit once, with only
# list items (and fences) copied from the body.
FIRST_HEADING=$(printf '%s\n' "$SECTION" | grep -m1 '^### ')
[[ "$FIRST_HEADING" == "### Breaking Changes" ]] || fail "breaking section is not first: $FIRST_HEADING"
EXPECTED_BREAKING='### Breaking Changes
- **Template syntax in a note (#8)**
  - Set `{{ .Env.HOME }}` style values by hand now.
- **Heading only, no title marker (#7)**
  - **Config**
  - Nested under a deeper heading.
- **Drop the legacy endpoint (#4)**
- **Fix a typo (#3)**
  - Lowercase heading, singular noun and level three still count.
    A paragraph note is kept, like the one on #595.
- **Rename the test mode key (#2)**
  - Operators, read this first.
  - **Test routes need their own key.** Hosts that set `ENVIRONMENT=test`
    no longer get the test routes. Set `SHISHO_TEST_MODE` instead.
  - Tab after the dash.
  - Numbered items count too.
  - So does this style.
  - Example config:
    ```yaml
    # a comment inside a fence is not a heading
    test_mode: true
    ```
  - A trailing paragraph becomes its own bullet.

### Features'
assert_contains "$SECTION" "$EXPECTED_BREAKING" "breaking section"
assert_not_contains "$SECTION" "must not leak" "later heading leaks into the breaking section"
assert_not_contains "$SECTION" "Fixes #" "Fixes line copied"
assert_not_contains "$SECTION" "Closes #" "Closes line copied"
assert_not_contains "$SECTION" "Co-authored-by" "trailer copied"
assert_not_contains "$SECTION" "not a real note" "fenced heading example started a section"
BREAKING_COUNT=$(printf '%s\n' "$SECTION" | grep -c '^- \*\*')
[[ "$BREAKING_COUNT" -eq 5 ]] || fail "expected 5 breaking commits, got $BREAKING_COUNT"

# No marker anywhere means no Breaking Changes heading at all.
QUIET=$(generate_changelog_section 0.0.2 v0.0.1..v0.0.1~0)
assert_not_contains "$QUIET" "### Breaking Changes" "empty range"
commit "[Docs] Plain docs change (#6)"
PLAIN=$(generate_changelog_section 0.0.3 HEAD~1..HEAD)
assert_not_contains "$PLAIN" "### Breaking Changes" "unmarked commit"

# The release header picks the block back out of CHANGELOG.md for the right
# version only.
CHANGELOG="$REPO/CHANGELOG.md"
{
    printf '# Changelog\n\n## [Unreleased]\n\n'
    printf '%s\n\n' "$SECTION"
    printf '## [0.0.20] - 2026-01-02\n\n### Breaking Changes\n- **Prefix neighbour**\n  - Must not be picked for 0.0.2.\n\n'
    printf '## [0.0.1] - 2026-01-01\n\n### Breaking Changes\n- **Old entry**\n  - Must not be picked for 0.0.2.\n'
} > "$CHANGELOG"
BLOCK=$(breaking_changes_from_changelog 0.0.2 "$CHANGELOG")
assert_contains "$BLOCK" "- **Rename the test mode key (#2)**" "changelog block"
assert_not_contains "$BLOCK" "Old entry" "changelog block picks another version"
assert_not_contains "$BLOCK" "Prefix neighbour" "changelog block matches a version prefix"
assert_not_contains "$BLOCK" "### " "changelog block keeps a heading"
assert_not_contains "$BLOCK" "Add dark mode" "changelog block runs past the breaking block"
NONE=$(breaking_changes_from_changelog 0.0.9 "$CHANGELOG")
[[ -z "$NONE" ]] || fail "changelog block for a missing version should be empty"

HEADER=$(cd "$SCRIPT_DIR/.." && ./scripts/release-notes-header.sh 0.0.2 "$CHANGELOG")
assert_contains "$HEADER" '## Shisho {{ .Tag }}' "header keeps the install block"
assert_contains "$HEADER" $'### Breaking Changes\n\nRead these before upgrading.\n\n- **Template syntax in a note (#8)**' "header carries the block"
assert_contains "$HEADER" '`{{"{{"}} .Env.HOME }}`' "header escapes template braces"
assert_not_contains "$HEADER" '`{{ .Env.HOME }}`' "header leaves a raw template action"
assert_contains "$HEADER" 'ghcr.io/shishobooks/shisho:{{ trimprefix .Tag "v" }}' "header keeps its own template actions"
HEADER_NONE=$(cd "$SCRIPT_DIR/.." && ./scripts/release-notes-header.sh 0.0.9 "$CHANGELOG" 2> "$REPO/stderr.txt")
assert_not_contains "$HEADER_NONE" "Breaking" "header without a block"
assert_contains "$(cat "$REPO/stderr.txt")" "::warning::" "missing entry warning"

if [[ "$FAILURES" -gt 0 ]]; then
    echo "$FAILURES failure(s)" >&2
    exit 1
fi
echo "changelog_test: ok"
