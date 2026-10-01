#!/usr/bin/env bash
# Changelog generation shared by scripts/release.sh and scripts/changelog_test.sh.
#
# Source this file, then call:
#   generate_changelog_section <version> <commit-range>
# It prints the Markdown section for CHANGELOG.md built from the commits in the
# range. Commit subjects follow the "[Category] Description" convention from
# AGENTS.md. Two markers flag a breaking change:
#   - a "!" right after the category in the subject: "[Fix]! Rename the key"
#   - a "## BREAKING CHANGES" heading in the commit body (the PR body, since
#     pull requests squash-merge with PR_BODY as the commit message), followed
#     by one bullet per change written for an operator who is upgrading
# A commit with either marker is listed under "### Breaking Changes" at the top
# of the section, with the body bullets under its subject, and still appears in
# its category list below.
#
# Bash 3 compatible (macOS ships 3.2): no associative arrays, no mapfile.

# breaking_section_from_body <sha>
# Prints the lines under a "## BREAKING CHANGES" heading in the commit body,
# up to the next heading or the end of the message. Prints nothing when the
# body has no such heading. The heading match is case-insensitive and accepts
# "BREAKING CHANGE" or "BREAKING CHANGES" at any heading level.
breaking_section_from_body() {
    local sha="$1"
    local in_section=false
    local line lower
    while IFS= read -r line; do
        if [[ "$line" =~ ^#{1,6}[[:space:]] ]]; then
            lower=$(printf '%s' "$line" | tr '[:upper:]' '[:lower:]')
            if [[ "$lower" =~ ^#{1,6}[[:space:]]+breaking[[:space:]]+changes?[[:space:]]*$ ]]; then
                in_section=true
                continue
            fi
            if [[ "$in_section" == "true" ]]; then
                break
            fi
            continue
        fi
        if [[ "$in_section" == "true" ]]; then
            printf '%s\n' "$line"
        fi
    done < <(git show -s --format=%B "$sha")
}

# indent_breaking_notes
# Reads section lines on stdin and prints them nested under a parent bullet:
# bullets and their wrapped continuation lines are indented two spaces, blank
# lines are dropped, and a plain paragraph becomes its own nested bullet.
indent_breaking_notes() {
    local line trimmed
    local in_bullet=false
    while IFS= read -r line; do
        trimmed="${line#"${line%%[![:space:]]*}"}"
        if [[ -z "$trimmed" ]]; then
            in_bullet=false
            continue
        fi
        if [[ "$trimmed" =~ ^[-*][[:space:]] ]]; then
            printf '  - %s\n' "${trimmed#[-*] }"
            in_bullet=true
        elif [[ "$in_bullet" == "true" ]]; then
            printf '    %s\n' "$trimmed"
        else
            printf '  - %s\n' "$trimmed"
            in_bullet=true
        fi
    done
}

generate_changelog_section() {
    local version="$1"
    local commit_range="$2"

    local commits_features=""
    local commits_bugfixes=""
    local commits_docs=""
    local commits_testing=""
    local commits_cicd=""
    local commits_other=""
    local commits_breaking=""

    local sha subject commit_cat marker commit_msg notes
    while IFS= read -r sha; do
        [[ -z "$sha" ]] && continue
        subject=$(git show -s --format=%s "$sha")
        marker=""

        # Extract category from "[Category]" or "[Category]!" format
        if [[ "$subject" =~ ^\[([^\]]+)\](!?)[[:space:]] ]]; then
            commit_cat="${BASH_REMATCH[1]}"
            marker="${BASH_REMATCH[2]}"
            commit_msg="${subject#\[$commit_cat\]$marker }"

            case "$commit_cat" in
                Frontend|Backend|Feature|Feat)
                    commits_features+="- $commit_msg"$'\n'
                    ;;
                Fix)
                    commits_bugfixes+="- $commit_msg"$'\n'
                    ;;
                Docs|Doc)
                    commits_docs+="- $commit_msg"$'\n'
                    ;;
                Test|E2E)
                    commits_testing+="- $commit_msg"$'\n'
                    ;;
                CI|CD)
                    commits_cicd+="- $commit_msg"$'\n'
                    ;;
                *)
                    commits_other+="- $commit_msg"$'\n'
                    ;;
            esac
        else
            commit_msg="$subject"
            commits_other+="- $commit_msg"$'\n'
        fi

        notes=$(breaking_section_from_body "$sha" | indent_breaking_notes)
        if [[ "$marker" == "!" || -n "$notes" ]]; then
            commits_breaking+="- **$commit_msg**"$'\n'
            if [[ -n "$notes" ]]; then
                commits_breaking+="$notes"$'\n'
            fi
        fi
    done < <(git log --pretty=tformat:"%H" $commit_range)

    local section="## [$version] - $(date +%Y-%m-%d)"$'\n'

    if [[ -n "$commits_breaking" ]]; then
        section+=$'\n'"### Breaking Changes"$'\n'
        section+="$commits_breaking"
    fi
    if [[ -n "$commits_features" ]]; then
        section+=$'\n'"### Features"$'\n'
        section+="$commits_features"
    fi
    if [[ -n "$commits_bugfixes" ]]; then
        section+=$'\n'"### Bug Fixes"$'\n'
        section+="$commits_bugfixes"
    fi
    if [[ -n "$commits_docs" ]]; then
        section+=$'\n'"### Documentation"$'\n'
        section+="$commits_docs"
    fi
    if [[ -n "$commits_testing" ]]; then
        section+=$'\n'"### Testing"$'\n'
        section+="$commits_testing"
    fi
    if [[ -n "$commits_cicd" ]]; then
        section+=$'\n'"### CI/CD"$'\n'
        section+="$commits_cicd"
    fi
    if [[ -n "$commits_other" ]]; then
        section+=$'\n'"### Other"$'\n'
        section+="$commits_other"
    fi

    printf '%s' "$section"
}

# breaking_changes_from_changelog <version> <changelog-file>
# Prints the "### Breaking Changes" block of the given version's entry in
# CHANGELOG.md, without its heading, or nothing when the entry has none. Used
# by scripts/release-notes-header.sh so the GitHub release page carries the
# same upgrade notes as the changelog.
breaking_changes_from_changelog() {
    local version="$1"
    local file="$2"
    local in_version=false
    local in_block=false
    local line
    while IFS= read -r line; do
        if [[ "$line" =~ ^##[[:space:]]+\[ ]]; then
            if [[ "$in_version" == "true" ]]; then
                break
            fi
            if [[ "$line" == "## [$version]"* ]]; then
                in_version=true
            fi
            continue
        fi
        [[ "$in_version" == "true" ]] || continue
        if [[ "$line" =~ ^###[[:space:]] ]]; then
            if [[ "$line" == "### Breaking Changes" ]]; then
                in_block=true
                continue
            fi
            if [[ "$in_block" == "true" ]]; then
                break
            fi
            continue
        fi
        if [[ "$in_block" == "true" ]]; then
            printf '%s\n' "$line"
        fi
    done < "$file"
}
