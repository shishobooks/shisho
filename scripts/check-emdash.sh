#!/usr/bin/env bash
# Fails when a line added on this branch contains an em-dash (U+2014).
#
# Only lines added relative to the merge base with origin/master (or master
# when there is no origin) are checked, because many existing files already
# contain em-dashes. Uncommitted and untracked files count as added, so the
# check runs before a commit too. Set EMDASH_BASE to compare against another
# ref.
#
# Run with: ./scripts/check-emdash.sh (or `mise lint:emdash`).
set -euo pipefail

EMDASH=$'\xe2\x80\x94'

cd "$(git rev-parse --show-toplevel)"

base_ref="${EMDASH_BASE:-}"
if [[ -z "$base_ref" ]]; then
    for candidate in origin/master master; do
        if git rev-parse --verify --quiet "$candidate^{commit}" > /dev/null; then
            base_ref="$candidate"
            break
        fi
    done
fi
if [[ -z "$base_ref" ]]; then
    echo "check-emdash: neither origin/master nor master exists; fetch master first" >&2
    exit 2
fi
if ! base=$(git merge-base HEAD "$base_ref"); then
    echo "check-emdash: no merge base between HEAD and $base_ref (is the clone shallow?)" >&2
    exit 2
fi

# excluded reports whether a path is exempt: the changelog quotes commit
# bodies, versioned docs are frozen snapshots, and fixtures and lockfiles are
# data rather than prose.
excluded() {
    case "$1" in
        CHANGELOG.md | website/versioned_docs/* | */testdata/* | testdata/* | */fixtures/* | fixtures/*)
            return 0
            ;;
        pnpm-lock.yaml | */pnpm-lock.yaml | package-lock.json | */package-lock.json | yarn.lock | */yarn.lock | go.sum | */go.sum | *.lock)
            return 0
            ;;
    esac
    return 1
}

# added_lines prints "path<TAB>line<TAB>text" for every added line: lines in
# the diff from the merge base to the working tree, then every line of each
# untracked file.
added_lines() {
    git -c core.quotePath=false diff --no-color --no-ext-diff -U0 "$base" -- |
        awk '
            /^diff --git / { inhunk = 0; next }
            !inhunk && /^\+\+\+ / { path = substr($0, 5); sub(/^b\//, "", path); next }
            /^@@ / {
                inhunk = 1
                split($3, parts, ",")
                line = substr(parts[1], 2) + 0
                next
            }
            inhunk && /^\+/ { printf "%s\t%d\t%s\n", path, line, substr($0, 2); line++ }
        '
    git -c core.quotePath=false ls-files --others --exclude-standard -z |
        while IFS= read -r -d '' file; do
            [[ -f "$file" ]] || continue
            awk -v path="$file" '{ printf "%s\t%d\t%s\n", path, NR, $0 }' "$file"
        done
}

# Collect into a file first: a process substitution would hide a git failure
# and let the check pass without checking anything.
added=$(mktemp)
trap 'rm -f "$added"' EXIT
added_lines > "$added"

found=0
while IFS=$'\t' read -r path line text; do
    [[ "$text" == *"$EMDASH"* ]] || continue
    excluded "$path" && continue
    echo "$path:$line: $text"
    found=$((found + 1))
done < "$added"

if [[ "$found" -gt 0 ]]; then
    echo "check-emdash: $found added line(s) contain an em-dash (U+2014). Rewrite them with a comma, colon, parentheses, or a new sentence." >&2
    exit 1
fi
