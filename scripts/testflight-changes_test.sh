#!/usr/bin/env bash
# Tests for scripts/testflight-changes.sh, which decides from a push's
# changed paths whether it changes the iOS app TestFlight ships.
# Run with: ./scripts/testflight-changes_test.sh (or `mise test:scripts`).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FILTER="$SCRIPT_DIR/testflight-changes.sh"

FAILURES=0
expect() {
    local want="$1" label="$2" paths="$3" got
    got=$(printf '%s' "$paths" | "$FILTER")
    if [[ "$got" != "$want" ]]; then
        echo "FAIL: $label: want $want, got $got" >&2
        FAILURES=$((FAILURES + 1))
    fi
}

# What the app is built from.
expect true "the app" "mobile/ios/Shisho/Reader/ReaderModel.swift"
expect true "the project" "mobile/ios/project.yml"
expect true "the build settings" "mobile/ios/Config/Shisho.xcconfig"
expect true "the API types' package" "mobile/ios/ShishoAPI/Package.swift"
expect true "foliate" "app/libraries/foliate/view.js"
expect true "the reader's fonts" "app/assets/fonts/Literata.woff2"
expect true "this workflow" ".github/workflows/testflight.yml"

# What it isn't. Go code only counts when the Swift generated from it
# changes, which takes a base to compare against.
expect false "the app's tests" "mobile/ios/ShishoTests/ListeningRulesTests.swift"
expect false "the API types' tests" "mobile/ios/ShishoAPI/Tests/ShishoAPITests/DecodeTests.swift"
expect false "an agent doc inside the app" "mobile/AGENTS.md"
expect false "a flow" "mobile/harness/flows/read_and_reopen.yaml"
expect false "the Android app" "mobile/android/api/build.gradle.kts"
expect false "the web app" "app/components/pages/AdminSettings.tsx"
expect false "server code" "pkg/mp4/chapters.go"
expect false "the iOS CI workflow" ".github/workflows/ios.yml"
expect false "no paths" ""

# One path that counts is enough.
expect true "server and app together" "pkg/reading/positions.go
mobile/ios/Shisho/Core/APIClient.swift"

# Given a base, the script lists the changes itself and compares the Swift
# the API types generate on each side, so only Go changes the app sees
# count. A stand-in generator writes one file from a Go file's types.
REPO=$(mktemp -d)
trap 'rm -rf "$REPO"' EXIT
(
    cd "$REPO"
    git init -q -b master .
    git config user.email test@example.com
    git config user.name test
    git config commit.gpgsign false
    mkdir -p pkg/books bin
    cat >bin/apptypes <<'EOF'
#!/usr/bin/env bash
# Writes the types of pkg/books/types.go as the Swift output.
set -euo pipefail
swift=""
while [ $# -gt 0 ]; do
    case "$1" in
        -swift) swift="$2"; shift 2 ;;
        *) shift ;;
    esac
done
mkdir -p "$swift"
{ grep '^type ' pkg/books/types.go; grep '^go ' go.mod; } >"$swift/Books.swift"
EOF
    chmod +x bin/apptypes
    printf 'package books\n\ntype Book struct{}\n' >pkg/books/types.go
    printf 'package books\n\nfunc List() {}\n' >pkg/books/service.go
    printf 'module example.com/books\n\ngo 1.25\n' >go.mod
    mkdir -p app/assets/fonts
    printf 'font\n' >app/assets/fonts/Literata.woff2
    git add -A
    git commit -q -m base
    git tag base
    printf 'package books\n\nfunc List() { _ = 1 }\n' >pkg/books/service.go
    git commit -q -am "server only"
    git tag server
    printf 'package books\n\ntype Book struct{}\n\ntype Series struct{}\n' >pkg/books/types.go
    git commit -q -am "a new type"
    git tag types
    printf 'module example.com/books\n\ngo 1.26\n' >go.mod
    git commit -q -am "a newer Go"
    git tag gomod
    mkdir -p app/vendor
    git mv app/assets/fonts/Literata.woff2 app/vendor/Literata.woff2
    git commit -q -m "a font moved out of the bundle"
    git tag moved
    git tag accented-base
    printf 'font\n' >"app/assets/fonts/Café.woff2"
    git add -A
    git commit -q -m "a font with an accent"
    git tag accented
    printf 'font\n' >'app/assets/fonts/Caf"e.woff2'
    git add -A
    git commit -q -m "a font with a quote"
)
check() {
    local want="$1" label="$2" base="$3" head="$4" got
    got=$(cd "$REPO" && git checkout -q "$head" &&
        TESTFLIGHT_APPTYPES="$REPO/bin/apptypes" "$FILTER" "$base")
    if [[ "$got" != "$want" ]]; then
        echo "FAIL: $label: want $want, got $got" >&2
        FAILURES=$((FAILURES + 1))
    fi
}
check false "Go that generates the same Swift" base server
check true "Go that generates new Swift" server types
check true "Go modules that generate new Swift" types gomod
check true "a rename out of a bundled folder" gomod moved
check true "a name with an accent" accented-base accented
check true "a name with a quote" accented master

# A generator that fails on either side fails the script, rather than
# reading as a change or no change. The head is generated in the clone
# itself, the base in a worktree beside it.
clone=$(cd "$REPO" && pwd -P)
for side in clone worktree; do
    # shellcheck disable=SC2016 # Expanded by the stand-in when it runs.
    if [ "$side" = clone ]; then fails='[ "$(pwd -P)" != "$CLONE" ]'; else fails='[ "$(pwd -P)" = "$CLONE" ]'; fi
    printf '#!/usr/bin/env bash\n%s || exit 1\nexec "%s" "$@"\n' "$fails" "$REPO/bin/apptypes" >"$REPO/bin/fails-in-$side"
    chmod +x "$REPO/bin/fails-in-$side"
    if got=$(cd "$REPO" && git checkout -q types &&
        CLONE="$clone" TESTFLIGHT_APPTYPES="$REPO/bin/fails-in-$side" "$FILTER" server 2>/dev/null); then
        echo "FAIL: a generator failing in the $side: want a failure, got $got" >&2
        FAILURES=$((FAILURES + 1))
    fi
done

# A base this clone doesn't have, such as one a force push dropped, builds
# rather than skipping a build that was needed.
check true "an unknown base" 0000000000000000000000000000000000000000 master

if [[ $FAILURES -gt 0 ]]; then
    echo "$FAILURES failure(s)" >&2
    exit 1
fi
echo "testflight-changes: all tests passed"
