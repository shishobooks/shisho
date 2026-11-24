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
expect true "the flows workflow" ".github/workflows/ios-flows.yml"
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
expect false "no paths" ""

# One path that counts is enough.
expect true "web and server together" "app/components/Foo.tsx
website/docs/x.md
pkg/reading/positions.go"

if [[ $FAILURES -gt 0 ]]; then
    echo "$FAILURES failure(s)" >&2
    exit 1
fi
echo "app-changes: all tests passed"
