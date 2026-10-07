#!/usr/bin/env bash
# Tests for scripts/check-emdash.sh against a throwaway git repository.
# Run with: ./scripts/check-emdash_test.sh (or `mise test:scripts`).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECK="$SCRIPT_DIR/check-emdash.sh"
EMDASH=$'\xe2\x80\x94'

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

# run_check runs the checker and stores its combined output and exit status.
OUT=""
RC=0
run_check() {
    RC=0
    OUT=$("$CHECK" 2>&1) || RC=$?
}

# Existing em-dashes on master are not the branch's concern.
printf 'old %s line\nplain\n' "$EMDASH" > old.txt
git add old.txt
git commit -q -m "base"

git checkout -q -b feature
run_check
[[ "$RC" -eq 0 ]] || fail "clean branch should pass, got $RC: $OUT"

# An untouched em-dash elsewhere in an edited file is not reported.
printf 'old %s line\nplain\nnew plain line\n' "$EMDASH" > old.txt
git commit -q -am "edit without em-dash"
run_check
[[ "$RC" -eq 0 ]] || fail "editing a file with an old em-dash should pass, got $RC: $OUT"

# A committed added line is reported with its path and line number.
printf 'line one\nline two %s here\n' "$EMDASH" > added.txt
git add added.txt
git commit -q -m "add em-dash"
run_check
[[ "$RC" -eq 1 ]] || fail "committed em-dash should fail with 1, got $RC"
assert_contains "$OUT" "added.txt:2: line two $EMDASH here" "committed em-dash location"
assert_not_contains "$OUT" "old.txt" "pre-existing em-dash"
git reset -q --hard HEAD~1

# An uncommitted edit to a tracked file counts.
printf 'old %s line\nplain\nnew plain line\nfresh %s edit\n' "$EMDASH" "$EMDASH" > old.txt
run_check
[[ "$RC" -eq 1 ]] || fail "uncommitted em-dash should fail with 1, got $RC"
assert_contains "$OUT" "old.txt:4: fresh $EMDASH edit" "uncommitted em-dash location"
assert_not_contains "$OUT" "old.txt:1:" "pre-existing line in an edited file"
git checkout -q -- old.txt

# An untracked file counts.
printf '%s untracked\n' "$EMDASH" > untracked.txt
run_check
[[ "$RC" -eq 1 ]] || fail "untracked em-dash should fail with 1, got $RC"
assert_contains "$OUT" "untracked.txt:1: $EMDASH untracked" "untracked em-dash location"
rm untracked.txt

# A line whose text starts with "++ " is content, not a diff header.
printf '++ %s plus\n' "$EMDASH" > plus.txt
git add plus.txt
git commit -q -m "plus line"
run_check
assert_contains "$OUT" "plus.txt:1: ++ $EMDASH plus" "line that looks like a header"
git reset -q --hard HEAD~1

# Excluded paths are skipped.
mkdir -p website/versioned_docs/version-1 pkg/thing/testdata e2e/fixtures app
for path in CHANGELOG.md website/versioned_docs/version-1/page.md pkg/thing/testdata/in.txt \
    e2e/fixtures/data.json pnpm-lock.yaml go.sum app/Cargo.lock; do
    printf 'skip %s me\n' "$EMDASH" > "$path"
done
git add -A
git commit -q -m "excluded paths"
run_check
[[ "$RC" -eq 0 ]] || fail "excluded paths should pass, got $RC: $OUT"

# origin/master wins over a stale local master: the line merged from
# origin/master would count as added against the older local master.
git checkout -q master
printf 'later %s on master\n' "$EMDASH" > later.txt
git add later.txt
git commit -q -m "master moves on"
git update-ref refs/remotes/origin/master HEAD
git reset -q --hard HEAD~1
git checkout -q feature
git merge -q --no-edit origin/master
run_check
[[ "$RC" -eq 0 ]] || fail "lines merged from origin/master should pass, got $RC: $OUT"
git update-ref -d refs/remotes/origin/master

# Without origin/master the stale local master is the base.
run_check
[[ "$RC" -eq 1 ]] || fail "falling back to master should see the merged line, got $RC: $OUT"
assert_contains "$OUT" "later.txt:1:" "master fallback"

# EMDASH_BASE overrides the base ref.
RC=0
OUT=$(EMDASH_BASE=HEAD~1 "$CHECK" 2>&1) || RC=$?
[[ "$RC" -eq 1 ]] || fail "EMDASH_BASE=HEAD~1 should see the merged line, got $RC: $OUT"
assert_contains "$OUT" "later.txt:1:" "EMDASH_BASE override"

# A missing base is an error, not a silent pass.
RC=0
OUT=$(EMDASH_BASE=does-not-exist "$CHECK" 2>&1) || RC=$?
[[ "$RC" -ne 0 && "$RC" -ne 1 ]] || fail "missing base should exit with a usage error, got $RC: $OUT"

if [[ "$FAILURES" -gt 0 ]]; then
    echo "$FAILURES failure(s)" >&2
    exit 1
fi
echo "check-emdash_test: ok"
