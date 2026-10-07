#!/usr/bin/env bash
# Fails when an agent instruction file (every AGENTS.md, CODING_STANDARDS.md,
# docs/agents/**/*.md) names a repo path, code identifier, or quoted section
# that does not exist. The checker lives in tools/checkdocrefs; intentional
# mentions of things that do not exist go in tools/checkdocrefs/allowlist.txt.
#
# Run with: ./scripts/check-docrefs.sh (or `mise lint:docrefs`).
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
# Outside a mise shell, go may not be on PATH; use the mise-pinned toolchain.
if command -v go > /dev/null; then
    exec go run ./tools/checkdocrefs "$@"
fi
exec mise exec -- go run ./tools/checkdocrefs "$@"
