#!/usr/bin/env bash
# Prints true when a pull request changes anything that can change what the
# Shisho app's checks test, else false (`app-changes` in
# .github/workflows/ci.yml). Given the base branch, it lists the changes
# against it itself, renames as both their old and new paths, so moving a
# file out of the app counts; without one, it reads paths from stdin, one
# per line.
#
# A denylist: the paths below can't affect the app, and anything else
# counts. The app is offline sync against the whole server, so a server
# package nobody thought to list must still run the flows; a path wrongly
# counted only costs a signoff.
set -euo pipefail

if [ $# -gt 0 ]; then
    # Into a variable first: a failed diff inside <(...) would pass for an
    # empty one, and skip the app's checks.
    paths=$(git diff --name-only --no-renames "$1...HEAD")
    exec <<<"$paths"
fi

changed=false
while IFS= read -r path || [ -n "$path" ]; do
    case "$path" in
        # The web reader's foliate and fonts, bundled into the iOS app
        # (mobile/ios/project.yml).
        app/libraries/foliate/* | app/assets/fonts/*) changed=true ;;
        # The web app, the docs and docs site, the KOReader plugin, and
        # tooling and packaging none of the app's checks use.
        app/* | public/* | assets/* | website/* | docs/* | e2e/* | demo/* | eslint-rules/* | \
            koreader-plugin/* | tools/koreaderharness/* | packages/plugin-sdk/* | \
            tools/checkdocrefs/* | tools/checkversions/* | tools/gotestsplit/* | \
            tools/koreaderpluginzip/* | tools/ruleguard/* | scripts/* | \
            .github/actions/setup-js/* | .github/actions/setup-playwright/* | \
            .golangci.yml | .air.toml | .wktr.yaml | .npmrc | .idea/* | eslint.config.js | shisho.dev.yaml | \
            .claude/* | .agents/* | .codex/* | .cursor/* | *.md | LICENSE | \
            index.html | components.json | package.json | pnpm-lock.yaml | pnpm-workspace.yaml | \
            vite.config.ts | vitest.config.ts | vitest.setup.ts | playwright.config.ts | \
            website.playwright.config.ts | tsconfig*.json | .prettierrc | .prettierignore | \
            Dockerfile | .dockerignore | docker-compose.yml | .goreleaser.yaml | shisho.example.yaml | \
            .github/workflows/docs.yml | .github/workflows/demo.yml | .github/workflows/testflight.yml) ;;
        "") ;;
        *) changed=true ;;
    esac
done
echo "$changed"
