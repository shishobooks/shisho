#!/usr/bin/env bash
# Tests for scripts/app-changes.sh, which decides from a pull request's
# changed paths whether the Shisho app's checks run.
# Run with: ./scripts/app-changes_test.sh (or `mise test:scripts`).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FILTER="$SCRIPT_DIR/app-changes.sh"

FAILURES=0
expect() {
    local want="$1" label="$2" paths="$3" got
    got=$(printf '%s' "$paths" | "$FILTER")
    if [[ "$got" != "$want" ]]; then
        echo "FAIL: $label: want $want, got $got" >&2
        FAILURES=$((FAILURES + 1))
    fi
}

# Server code counts, a package no one listed included.
expect true "a media parser" "pkg/mp4/chapters.go"
expect true "a package added later" "pkg/somethingnew/thing.go"
expect true "the server's entry point" "cmd/api/main.go"
expect true "the app" "mobile/ios/Shisho/Reader/ReaderModel.swift"
expect true "the harness" "tools/appharness/ios.go"
expect true "a flow" "mobile/harness/flows/read_and_reopen.yaml"
expect true "Go modules" "go.sum"
expect true "the CI workflow" ".github/workflows/ci.yml"
expect true "the iOS workflow" ".github/workflows/ios.yml"
# The web reader's foliate and fonts are bundled into the iOS app.
expect true "foliate" "app/libraries/foliate/view.js"
expect true "the reader's fonts" "app/assets/fonts/Literata.woff2"

# Nothing the app's checks test.
expect false "the web app" "app/components/pages/AdminSettings.tsx"
expect false "the docs site" "website/docs/configuration.md"
expect false "an agent doc inside mobile" "mobile/AGENTS.md"
expect false "the web e2e tests" "e2e/reader.spec.ts"
expect false "the KOReader plugin" "koreader-plugin/shisho.koplugin/main.lua"
expect false "JS dependencies" "pnpm-lock.yaml"
expect false "the docs workflow" ".github/workflows/docs.yml"
expect false "the TestFlight workflow" ".github/workflows/testflight.yml"
expect false "no paths" ""

# One path that counts is enough.
expect true "web and server together" "app/components/Foo.tsx
website/docs/x.md
pkg/reading/positions.go"

# Tooling none of the app's checks use.
expect false "a lint tool" "tools/checkdocrefs/main.go"
expect false "the Go lint config" ".golangci.yml"
expect false "the JS lint config" "eslint.config.js"
expect false "a repo script" "scripts/check-emdash.sh"
expect false "the web's CI setup" ".github/actions/setup-js/action.yml"
expect true "the iOS CI setup" ".github/actions/setup-ios/action.yml"

# Given the base branch, the script lists the pull request's changes itself.
# A rename must count its old path too: moving foliate out of the folder the
# iOS app bundles leaves a new path that's ruled out.
REPO=$(mktemp -d)
trap 'rm -rf "$REPO"' EXIT
(
    cd "$REPO"
    git init -q -b master .
    git config user.email test@example.com
    git config user.name test
    git config commit.gpgsign false
    mkdir -p app/libraries/foliate
    printf 'export const view = 1\n' >app/libraries/foliate/view.js
    git add -A
    git commit -q -m base
    git checkout -q -b moved
    mkdir -p app/vendor
    git mv app/libraries/foliate/view.js app/vendor/view.js
    git commit -q -m moved
)
got=$(cd "$REPO" && "$FILTER" master)
if [[ "$got" != true ]]; then
    echo "FAIL: a rename out of the bundled folder: want true, got $got" >&2
    FAILURES=$((FAILURES + 1))
fi

# A diff that fails must fail the script, not read as no changes and skip
# the app's checks.
if got=$(cd "$REPO" && "$FILTER" no-such-base 2>/dev/null); then
    echo "FAIL: a missing base: want a failure, got $got" >&2
    FAILURES=$((FAILURES + 1))
fi

if [[ $FAILURES -gt 0 ]]; then
    echo "$FAILURES failure(s)" >&2
    exit 1
fi
echo "app-changes: all tests passed"
