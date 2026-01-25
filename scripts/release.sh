#!/usr/bin/env bash
set -euo pipefail

# Release script for Shisho
# Usage: ./scripts/release.sh <version>
# Example: ./scripts/release.sh 0.1.0

VERSION="${1:-}"

if [[ -z "$VERSION" ]]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 0.1.0"
    exit 1
fi

# Ensure version doesn't start with 'v'
VERSION="${VERSION#v}"
TAG="v$VERSION"

# Check for uncommitted changes
if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "Error: You have uncommitted changes. Please commit or stash them first."
    exit 1
fi

# Check we're on master branch
CURRENT_BRANCH=$(git branch --show-current)
if [[ "$CURRENT_BRANCH" != "master" ]]; then
    echo "Error: You must be on the master branch to create a release."
    echo "Current branch: $CURRENT_BRANCH"
    exit 1
fi

# Check tag doesn't already exist
if git rev-parse "$TAG" >/dev/null 2>&1; then
    echo "Error: Tag $TAG already exists."
    exit 1
fi

echo "Creating release $TAG..."

# Get the previous tag for changelog generation
PREV_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "")

# Generate changelog entries from commits since last tag
echo "Generating changelog..."

CHANGELOG_ENTRIES=""
if [[ -n "$PREV_TAG" ]]; then
    COMMIT_RANGE="$PREV_TAG..HEAD"
else
    COMMIT_RANGE="HEAD"
fi

# Group commits by category
declare -A CATEGORIES
CATEGORIES=(
    ["Features"]="Frontend|Backend|Feature|Feat"
    ["Bug Fixes"]="Fix"
    ["Documentation"]="Docs|Doc"
    ["Testing"]="Test|E2E"
    ["CI/CD"]="CI|CD"
    ["Other"]=".*"
)

# Read commits into arrays by category
declare -A CATEGORY_COMMITS
for category in "Features" "Bug Fixes" "Documentation" "Testing" "CI/CD" "Other"; do
    CATEGORY_COMMITS[$category]=""
done

while IFS= read -r commit; do
    [[ -z "$commit" ]] && continue

    # Extract category from [Category] format
    if [[ "$commit" =~ ^\[([^\]]+)\] ]]; then
        commit_cat="${BASH_REMATCH[1]}"
        commit_msg="${commit#\[$commit_cat\] }"

        matched=false
        for category in "Features" "Bug Fixes" "Documentation" "Testing" "CI/CD"; do
            pattern="${CATEGORIES[$category]}"
            if [[ "$commit_cat" =~ ^($pattern)$ ]]; then
                CATEGORY_COMMITS[$category]+="- $commit_msg"$'\n'
                matched=true
                break
            fi
        done

        if [[ "$matched" == "false" ]]; then
            CATEGORY_COMMITS["Other"]+="- $commit_msg"$'\n'
        fi
    else
        CATEGORY_COMMITS["Other"]+="- $commit"$'\n'
    fi
done < <(git log --pretty=format:"%s" $COMMIT_RANGE)

# Build changelog section
CHANGELOG_SECTION="## [$VERSION] - $(date +%Y-%m-%d)"$'\n'

for category in "Features" "Bug Fixes" "Documentation" "Testing" "CI/CD" "Other"; do
    if [[ -n "${CATEGORY_COMMITS[$category]}" ]]; then
        CHANGELOG_SECTION+=$'\n'"### $category"$'\n'
        CHANGELOG_SECTION+="${CATEGORY_COMMITS[$category]}"
    fi
done

# Update CHANGELOG.md
echo "Updating CHANGELOG.md..."
CHANGELOG_FILE="CHANGELOG.md"

# Read existing changelog
EXISTING_CHANGELOG=$(cat "$CHANGELOG_FILE")

# Find the [Unreleased] section and insert new version after it
NEW_CHANGELOG=$(echo "$EXISTING_CHANGELOG" | sed "/^## \[Unreleased\]/a\\
\\
$CHANGELOG_SECTION")

echo "$NEW_CHANGELOG" > "$CHANGELOG_FILE"

# Update package versions
echo "Updating package.json..."
npm version "$VERSION" --no-git-tag-version

echo "Updating packages/plugin-types/package.json..."
cd packages/plugin-types
npm version "$VERSION" --no-git-tag-version
cd ../..

# Commit changes
echo "Committing changes..."
git add CHANGELOG.md package.json packages/plugin-types/package.json
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
