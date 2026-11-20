#!/usr/bin/env bash
# Keeps the Shisho app's Swift under Swift 6's data-race checks, which turn
# a race into a compile error instead of a rare crash.
#
# It fails when:
#   - the app (mobile/ios/project.yml) or the API package
#     (mobile/ios/ShishoAPI/Package.swift) builds in a language mode older
#     than Swift 6, or a target in the package opts back into one;
#   - Swift code opts out of the checks with @unchecked Sendable,
#     nonisolated(unsafe), @preconcurrency, or assumeIsolated, unless the
#     comment right above it says why with "// concurrency-exception: <reason>".
#
# Generated and build files are gitignored, so they're never checked.
#
# Run with: ./scripts/check-swift-concurrency.sh (or `mise lint:swiftconcurrency`).
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

found=0
report() {
    echo "$1"
    found=$((found + 1))
}

project=mobile/ios/project.yml
if [[ -f "$project" ]]; then
    while IFS= read -r match; do
        version=$(sed -E 's/.*SWIFT_VERSION:[[:space:]]*"?([0-9]+).*/\1/' <<< "${match#*:}")
        if [[ "$version" -lt 6 ]]; then
            report "$project:$match"
        fi
    done < <(grep -n 'SWIFT_VERSION:' "$project" || true)
fi

package=mobile/ios/ShishoAPI/Package.swift
if [[ -f "$package" ]]; then
    tools=$(sed -nE '1s|^// swift-tools-version:[[:space:]]*([0-9]+).*|\1|p' "$package")
    if [[ -z "$tools" || "$tools" -lt 6 ]]; then
        report "$package:1: $(head -n 1 "$package")"
    fi
    while IFS= read -r match; do
        report "$package:$match"
    done < <(grep -nE '\.v[45]([^0-9]|$)|swiftLanguageMode|swiftLanguageVersions' "$package" || true)
fi

escape='@unchecked[[:space:]]+Sendable|nonisolated[(]unsafe[)]|@preconcurrency|assumeIsolated'
while IFS= read -r -d '' file; do
    [[ -f "$file" ]] || continue
    while IFS= read -r match; do
        report "$file:$match"
    done < <(awk -v pattern="$escape" '
        # excused: the comment block right above this line gave a reason.
        /^[[:space:]]*\/\// { if ($0 ~ /\/\/ concurrency-exception:/) excused = 1; next }
        $0 ~ pattern && !excused { printf "%d: %s\n", NR, $0 }
        { excused = 0 }
    ' "$file")
done < <(git -c core.quotePath=false ls-files -z --cached --others --exclude-standard -- 'mobile/*.swift')

if [[ "$found" -gt 0 ]]; then
    echo "check-swift-concurrency: $found problem(s). The app and the ShishoAPI package build in Swift 6 language mode, so data races are compile errors. Fix the code so the compiler can prove it safe; when that's impossible, say why in a \"// concurrency-exception: <why this can't race>\" comment right above the escape hatch." >&2
    exit 1
fi
