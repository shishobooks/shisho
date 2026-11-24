#!/usr/bin/env bash
# Reads a pull request's changed paths, one per line, and prints true when
# any of them can change what the Shisho app's checks test, else false
# (`app-changes` in .github/workflows/ci.yml).
#
# A denylist: the paths below can't affect the app, and anything else
# counts. The app is offline sync against the whole server, so a server
# package nobody thought to list must still run the flows; a path wrongly
# counted only costs a signoff.
set -euo pipefail

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
            .claude/* | .agents/* | .codex/* | .cursor/* | *.md | LICENSE | \
            index.html | components.json | package.json | pnpm-lock.yaml | pnpm-workspace.yaml | \
            vite.config.ts | vitest.config.ts | vitest.setup.ts | playwright.config.ts | \
            website.playwright.config.ts | tsconfig*.json | .prettierrc | .prettierignore | \
            Dockerfile | .dockerignore | docker-compose.yml | .goreleaser.yaml | shisho.example.yaml | \
            .github/workflows/docs.yml | .github/workflows/demo.yml) ;;
        "") ;;
        *) changed=true ;;
    esac
done
echo "$changed"
