#!/usr/bin/env bash
# Prints true when HEAD changes the iOS app that TestFlight ships since the
# given commit, the last one uploaded, else false
# (.github/workflows/testflight.yml). Given that commit, it lists the
# changes itself, renames as both their old and new paths, and compares the
# Swift API types generated on each side; without one, it reads paths from
# stdin, one per line, and answers from the paths alone.
#
# An allowlist, unlike scripts/app-changes.sh: a build nobody needed puts a
# needless update on testers' phones, and a server change reaches the app
# only through the API types, which the comparison catches.
set -euo pipefail

app_path() {
    case "$1" in
        mobile/ios/ShishoTests/* | mobile/ios/ShishoAPI/Tests/* | *.md) return 1 ;;
        # The app, and the web reader's foliate and fonts it bundles: the
        # folders outside mobile/ios that mobile/ios/project.yml names.
        mobile/ios/* | app/libraries/foliate/* | app/assets/fonts/*) return 0 ;;
        .github/workflows/testflight.yml) return 0 ;;
        *) return 1 ;;
    esac
}

if [ $# -eq 0 ]; then
    while IFS= read -r path || [ -n "$path" ]; do
        if app_path "$path"; then
            echo true
            exit 0
        fi
    done
    echo false
    exit 0
fi

base="$1"
# A base this clone doesn't have, such as one a force push dropped, can't
# be compared, so it builds.
if ! git cat-file -e "$base^{commit}" 2>/dev/null; then
    echo true
    exit 0
fi

# Into a variable first: a failed diff inside <(...) would pass for an
# empty one, and skip a build. Unquoted, so a name with an accent still
# matches its folder.
paths=$(git -c core.quotePath=false diff --name-only --no-renames "$base" HEAD)
go_changed=false
while IFS= read -r path; do
    if app_path "$path"; then
        echo true
        exit 0
    fi
    case "$path" in
        # go.mod too: a dependency's types can reach the API types.
        *.go | go.mod | go.sum | tygo.yaml) go_changed=true ;;
    esac
done <<<"$paths"
if [ "$go_changed" = false ]; then
    echo false
    exit 0
fi

# Each side generates with its own generator, as its own build would.
# TESTFLIGHT_APPTYPES stands in for it in the script's tests.
tmp=$(mktemp -d)
cleanup() {
    git worktree remove --force "$tmp/base" >/dev/null 2>&1 || true
    rm -rf "$tmp"
}
trap cleanup EXIT
generate() {
    local dir="$1" out="$2"
    (
        cd "$dir"
        if [ -n "${TESTFLIGHT_APPTYPES:-}" ]; then
            "$TESTFLIGHT_APPTYPES" -swift "$out" -kotlin "$tmp/kotlin"
        else
            go run ./tools/apptypes -swift "$out" -kotlin "$tmp/kotlin"
        fi
    ) >/dev/null
}
git worktree add --quiet --detach "$tmp/base" "$base"
generate "$tmp/base" "$tmp/swift-base"
generate "$(git rev-parse --show-toplevel)" "$tmp/swift-head"
if diff -r "$tmp/swift-base" "$tmp/swift-head" >/dev/null; then
    echo false
else
    echo true
fi
