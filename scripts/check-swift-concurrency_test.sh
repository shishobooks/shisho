#!/usr/bin/env bash
# Tests for scripts/check-swift-concurrency.sh against a throwaway git
# repository.
# Run with: ./scripts/check-swift-concurrency_test.sh (or `mise test:scripts`).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECK="$SCRIPT_DIR/check-swift-concurrency.sh"

FAILURES=0
fail() {
    echo "FAIL: $1" >&2
    FAILURES=$((FAILURES + 1))
}

REPO=$(mktemp -d)
trap 'rm -rf "$REPO"' EXIT
cd "$REPO"
git init -q -b master .
mkdir -p mobile/ios/ShishoAPI mobile/ios/Shisho

OUT=""
RC=0
run_check() {
    RC=0
    OUT=$("$CHECK" 2>&1) || RC=$?
}
expect_pass() {
    run_check
    [[ "$RC" -eq 0 ]] || fail "$1: expected a pass, got $RC: $OUT"
}
expect_fail() {
    local label="$1" needle="$2"
    run_check
    [[ "$RC" -eq 1 ]] || fail "$label: expected a failure, got $RC: $OUT"
    [[ "$OUT" == *"$needle"* ]] || fail "$label: expected to find \"$needle\" in: $OUT"
}

good_project() {
    printf 'settings:\n  base:\n    SWIFT_VERSION: "6.0"\n' > mobile/ios/project.yml
}
good_package() {
    printf '// swift-tools-version:6.0\nlet package = Package(platforms: [.iOS(.v17), .macOS(.v14)])\n' > mobile/ios/ShishoAPI/Package.swift
}
good_project
good_package
printf 'struct Fine: Sendable {}\n' > mobile/ios/Shisho/Fine.swift
expect_pass "Swift 6 everywhere, no escape hatches"

# Language modes.
printf 'settings:\n  base:\n    SWIFT_VERSION: "5.0"\n' > mobile/ios/project.yml
expect_fail "project in Swift 5" 'SWIFT_VERSION: "5.0"'
printf 'targets:\n  Shisho:\n    settings:\n      base:\n        SWIFT_VERSION: 5\n' > mobile/ios/project.yml
expect_fail "a target in Swift 5" "SWIFT_VERSION: 5"
good_project

printf '// swift-tools-version:5.9\n' > mobile/ios/ShishoAPI/Package.swift
expect_fail "package tools 5.9" "swift-tools-version:5.9"
printf '// swift-tools-version:6.0\n.target(name: "A", swiftSettings: [.swiftLanguageMode(.v5)])\n' > mobile/ios/ShishoAPI/Package.swift
expect_fail "a package target in Swift 5" "swiftLanguageMode"
good_package

# Escape hatches, in tracked and untracked files alike.
printf 'final class Box: @unchecked Sendable {}\n' > mobile/ios/Shisho/Box.swift
expect_fail "@unchecked Sendable" "mobile/ios/Shisho/Box.swift:1:"
printf 'nonisolated(unsafe) var shared = 0\n' > mobile/ios/Shisho/Box.swift
git add mobile/ios/Shisho/Box.swift
expect_fail "nonisolated(unsafe)" "nonisolated(unsafe)"
printf '@preconcurrency import Foundation\n' > mobile/ios/Shisho/Box.swift
expect_fail "@preconcurrency" "@preconcurrency"
printf 'func f() {\n    MainActor.assumeIsolated { }\n}\n' > mobile/ios/Shisho/Box.swift
expect_fail "assumeIsolated, with its line number" "Box.swift:2:"

# A reason on the line above lets one through; a reason elsewhere doesn't.
printf '// concurrency-exception: only ever touched under the lock below.\nfinal class Box: @unchecked Sendable {}\n' > mobile/ios/Shisho/Box.swift
expect_pass "escape hatch with a reason above"
printf '// concurrency-exception: reason\n\nfinal class Box: @unchecked Sendable {}\n' > mobile/ios/Shisho/Box.swift
expect_fail "reason not directly above" "Box.swift:3:"
printf '/// A box.\n// concurrency-exception: a reason long enough\n// to wrap onto a second line.\n    final class Box: @unchecked Sendable {}\n' > mobile/ios/Shisho/Box.swift
expect_pass "reason in the comment block above"
printf '// concurrency-exception: reason\nlet x = 1\nfinal class Box: @unchecked Sendable {}\n' > mobile/ios/Shisho/Box.swift
expect_fail "code between the reason and the escape hatch" "Box.swift:3:"

# A comment that only mentions an escape hatch isn't one.
printf '// Never use @preconcurrency here.\nimport Foundation\n' > mobile/ios/Shisho/Box.swift
expect_pass "escape hatch named in a comment"

# Ignored files, such as generated types, aren't checked.
printf 'Generated/\n' > .gitignore
mkdir -p mobile/ios/Generated
printf 'final class G: @unchecked Sendable {}\n' > mobile/ios/Generated/G.swift
git rm -qf --cached mobile/ios/Shisho/Box.swift
rm mobile/ios/Shisho/Box.swift
expect_pass "gitignored Swift"

if [[ "$FAILURES" -gt 0 ]]; then
    echo "$FAILURES failure(s)" >&2
    exit 1
fi
echo "check-swift-concurrency: all tests passed"
