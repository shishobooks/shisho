#!/usr/bin/env bash
set -euo pipefail

# Release script for Shisho
# Usage: ./scripts/release.sh <version> [--dry-run]
# Example: ./scripts/release.sh 0.1.0
# Example: ./scripts/release.sh 0.1.0 --dry-run

DRY_RUN=false
VERSION=""

# Parse arguments
for arg in "$@"; do
    case "$arg" in
        --dry-run)
            DRY_RUN=true
            ;;
        *)
            if [[ -z "$VERSION" ]]; then
                VERSION="$arg"
            fi
            ;;
    esac
done

if [[ -z "$VERSION" ]]; then
    echo "Usage: $0 <version> [--dry-run]"
    echo "Example: $0 0.1.0"
    echo "Example: $0 0.1.0 --dry-run"
    exit 1
fi

# Ensure version doesn't start with 'v'
VERSION="${VERSION#v}"
TAG="v$VERSION"

# Check for uncommitted changes
if ! git diff --quiet || ! git diff --cached --quiet; then
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "Warning: You have uncommitted changes (ignored in dry-run mode)."
    else
        echo "Error: You have uncommitted changes. Please commit or stash them first."
        exit 1
    fi
fi

# Check we're on master branch
CURRENT_BRANCH=$(git branch --show-current)
if [[ "$CURRENT_BRANCH" != "master" ]]; then
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "Warning: Not on master branch (ignored in dry-run mode). Current branch: $CURRENT_BRANCH"
    else
        echo "Error: You must be on the master branch to create a release."
        echo "Current branch: $CURRENT_BRANCH"
        exit 1
    fi
fi

# Check tag doesn't already exist
if git rev-parse "$TAG" >/dev/null 2>&1; then
    echo "Error: Tag $TAG already exists."
    exit 1
fi

if [[ "$DRY_RUN" == "true" ]]; then
    echo "=== DRY RUN: Creating release $TAG ==="
else
    echo "Creating release $TAG..."
fi

# Get the previous tag for changelog generation
PREV_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")

# Generate the changelog entry from commits since the last tag. The generator
# lives in scripts/lib/changelog.sh so scripts/changelog_test.sh can exercise it
# against a fixture repository. Commits marked breaking (a "!" after the
# category, or a "## BREAKING CHANGES" section in the commit body) are listed
# first with their upgrade notes; see docs/agents/releases.md.
echo "Generating changelog..."

if [[ -n "$PREV_TAG" ]]; then
    COMMIT_RANGE="$PREV_TAG..HEAD"
else
    COMMIT_RANGE="HEAD"
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/changelog.sh
source "$SCRIPT_DIR/lib/changelog.sh"

CHANGELOG_SECTION=$(generate_changelog_section "$VERSION" "$COMMIT_RANGE")

# In dry-run mode, show what would be added to changelog and exit
if [[ "$DRY_RUN" == "true" ]]; then
    echo ""
    echo "=== Changelog entry that would be added ==="
    echo "$CHANGELOG_SECTION"
    echo "=== End changelog entry ==="
    echo ""
    echo "Would update:"
    echo "  - CHANGELOG.md"
    echo "  - package.json -> $VERSION"
    echo "  - packages/plugin-sdk/package.json -> $VERSION"
    echo "  - website/versioned_docs + website/versions.json"
    echo ""
    echo "Would commit: [Release] $TAG"
    echo "Would create tag: $TAG"
    echo "Would push: master and $TAG to origin"
    echo ""
    echo "=== DRY RUN COMPLETE ==="
    exit 0
fi

# Update CHANGELOG.md
echo "Updating CHANGELOG.md..."
CHANGELOG_FILE="CHANGELOG.md"

# Insert new version after [Unreleased] section using pure bash
# (awk -v doesn't handle multi-line strings, and BSD sed differs from GNU sed)
{
    found=false
    while IFS= read -r line; do
        echo "$line"
        if [[ "$line" =~ ^##\ \[Unreleased\] ]] && [[ "$found" == "false" ]]; then
            echo ""
            echo "$CHANGELOG_SECTION"
            found=true
        fi
    done < "$CHANGELOG_FILE"
} > "$CHANGELOG_FILE.tmp" && mv "$CHANGELOG_FILE.tmp" "$CHANGELOG_FILE"

# Version docs site snapshot
if [[ -d "website" ]]; then
    echo "Versioning docs website..."
    (
        cd website
        pnpm install --frozen-lockfile
        pnpm docs:version "$VERSION"
    )
fi

# Update package versions
echo "Updating package.json..."
npm version "$VERSION" --no-git-tag-version

echo "Updating packages/plugin-sdk/package.json..."
cd packages/plugin-sdk
npm version "$VERSION" --no-git-tag-version
cd ../..

# Commit changes
echo "Committing changes..."
git add CHANGELOG.md package.json packages/plugin-sdk/package.json
if [[ -d "website" ]]; then
    git add -A website
fi
git commit -m "[Release] $TAG"

# Create tag
echo "Creating tag $TAG..."
git tag -a "$TAG" -m "Release $TAG"

# Push
echo "Pushing to origin..."
git push origin master
git push origin "$TAG"

echo ""
echo "Release $TAG created successfully!"
echo "GitHub Actions will now build and publish the release."
echo ""
echo "View the release at: https://github.com/shishobooks/shisho/releases/tag/$TAG"
