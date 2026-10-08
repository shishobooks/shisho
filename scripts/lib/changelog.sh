#!/usr/bin/env bash
# Changelog generation shared by scripts/release.sh, scripts/release-notes-header.sh
# and scripts/changelog_test.sh.
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
# Prints the content under a "## BREAKING CHANGES" heading in the commit body
# and returns 0 when the heading exists, 1 when it does not (the heading alone
# marks the commit as breaking, even with nothing under it). The match is
# case-insensitive, accepts "BREAKING CHANGE" or "BREAKING CHANGES", and works
# at any heading level. The section ends at the next heading of the same or a
# shallower level; a deeper heading inside it is kept as a bold bullet. Lines
# inside fenced code blocks are never treated as headings, so an example of the
# heading in a fence does not start or end a section.
breaking_section_from_body() {
    local sha="$1"
    local in_section=false
    local found=1
    local in_fence=false
    local section_level=0
    local line lower hashes level
    while IFS= read -r line; do
        if [[ "$line" =~ ^[[:space:]]*(\`\`\`|~~~) ]]; then
            if [[ "$in_fence" == "true" ]]; then in_fence=false; else in_fence=true; fi
            if [[ "$in_section" == "true" ]]; then
                printf '%s\n' "$line"
            fi
            continue
        fi
        if [[ "$in_fence" == "false" && "$line" =~ ^(#{1,6})[[:space:]] ]]; then
            hashes="${BASH_REMATCH[1]}"
            level=${#hashes}
            lower=$(printf '%s' "$line" | tr '[:upper:]' '[:lower:]')
            if [[ "$in_section" == "false" ]]; then
                if [[ "$lower" =~ ^#{1,6}[[:space:]]+breaking[[:space:]]+changes?[[:space:]]*$ ]]; then
                    in_section=true
                    found=0
                    section_level=$level
                fi
                continue
            fi
            if (( level <= section_level )); then
                break
            fi
            # A subheading inside the section becomes a bold bullet.
            line="${line#"${hashes}"}"
            line="${line#"${line%%[![:space:]]*}"}"
            printf -- '- **%s**\n' "$line"
            continue
        fi
        if [[ "$in_section" == "true" ]]; then
            printf '%s\n' "$line"
        fi
    done < <(git show -s --format=%B "$sha")
    return $found
}

# indent_breaking_notes
# Reads section lines on stdin and prints them nested under a parent bullet,
# two spaces per level: a top-level item is printed as "  - text". Nesting
# follows CommonMark, so the changelog matches the PR preview the author
# checked: "-", "*" or "1." items become "- " items, and an item is a child of
# the nearest open item whose content column (its indent plus the marker and
# one space, so 2 for "- " and 3 for "1. ") its indent reaches; otherwise it is
# a sibling. Tabs count to the next multiple of four. Wrapped continuation
# lines, indented or not, stay with their item. After a blank line, an
# indented paragraph that reaches an open item's content column stays in that
# item: printed at the item's text column, so it joins the item's last
# paragraph, or after a blank line as a separate paragraph when a child item
# sits between them, so it does not join the child. Any other paragraph
# becomes its own top-level item (older PR bodies wrote the note as a
# paragraph). A "### Subheading" that breaking_section_from_body turned into
# "- **Subheading**" is a top-level item like any other. Fenced code blocks
# are copied inside the item their indent reaches. Other blank lines are
# dropped so the list stays tight. Issue references such as "Closes #12" and
# trailers such as "Co-authored-by:" are dropped, because squash bodies end
# with them and they are not upgrade notes.
indent_breaking_notes() {
    local line lead trimmed lower i indent
    local in_bullet=false
    local in_fence=false
    local fence_indent="" fence_pad=""
    # Content column of each open item, outermost first; n of them are open.
    local open_content=()
    local n=0
    # Depth of the current item and of the item the last printed line is in.
    local depth=0 last_depth=0
    local pad="  "
    # Kept in variables because a ")" inside a bracket expression confuses
    # the [[ ]] parser when the pattern is written inline.
    local item_re='^([-*]|[0-9]+[.)])[[:space:]]+(.*)$'
    local issue_ref_re='^(close[sd]?|fix(e[sd])?|resolve[sd]?|refs?)[[:space:]:]+(#|https?://)'
    local trailer_re='^[a-z][a-z-]*-by:[[:space:]]'
    while IFS= read -r line; do
        if [[ "$in_fence" == "true" ]]; then
            if [[ "$line" =~ ^[[:space:]]*(\`\`\`|~~~) ]]; then
                in_fence=false
                printf '%s%s\n' "$fence_pad" "${line#"${line%%[![:space:]]*}"}"
            else
                printf '%s%s\n' "$fence_pad" "${line#"$fence_indent"}"
            fi
            continue
        fi
        lead="${line%%[![:space:]]*}"
        trimmed="${line#"$lead"}"
        if [[ -z "$trimmed" ]]; then
            in_bullet=false
            continue
        fi
        indent=0
        for (( i = 0; i < ${#lead}; i++ )); do
            if [[ "${lead:i:1}" == $'\t' ]]; then
                indent=$(( (indent / 4 + 1) * 4 ))
            else
                indent=$(( indent + 1 ))
            fi
        done
        if [[ "$trimmed" =~ ^(\`\`\`|~~~) ]]; then
            # The fence's own indentation is removed from its content so the
            # block nests inside its item wherever it was written. Text right
            # after the closing fence continues that item.
            in_fence=true
            fence_indent="$lead"
            while (( n > 0 )) && (( indent < open_content[n-1] )); do n=$(( n - 1 )); done
            depth=$(( n > 0 ? n - 1 : 0 ))
            printf -v pad '%*s' $(( 2 + depth * 2 )) ''
            fence_pad="$pad  "
            printf '%s%s\n' "$fence_pad" "$trimmed"
            in_bullet=true
            last_depth=$depth
            continue
        fi
        if [[ "$trimmed" =~ $item_re ]]; then
            while (( n > 0 )) && (( indent < open_content[n-1] )); do n=$(( n - 1 )); done
            depth=$n
            open_content[n]=$(( indent + ${#BASH_REMATCH[1]} + 1 ))
            n=$(( n + 1 ))
            printf -v pad '%*s' $(( 2 + depth * 2 )) ''
            printf '%s- %s\n' "$pad" "${BASH_REMATCH[2]}"
            in_bullet=true
            last_depth=$depth
            continue
        fi
        lower=$(printf '%s' "$trimmed" | tr '[:upper:]' '[:lower:]')
        if [[ "$lower" =~ $issue_ref_re || "$lower" =~ $trailer_re ]]; then
            in_bullet=false
            continue
        fi
        if [[ "$in_bullet" == "true" ]]; then
            printf '%s  %s\n' "$pad" "$trimmed"
            continue
        fi
        # A paragraph after a blank line.
        while (( n > 0 )) && (( indent < open_content[n-1] )); do n=$(( n - 1 )); done
        if (( n > 0 )); then
            depth=$(( n - 1 ))
            printf -v pad '%*s' $(( 2 + depth * 2 )) ''
            if (( depth < last_depth )); then
                printf '\n'
            fi
            printf '%s  %s\n' "$pad" "$trimmed"
        else
            depth=0
            pad="  "
            printf '  - %s\n' "$trimmed"
        fi
        in_bullet=true
        last_depth=$depth
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

    local sha subject commit_cat marker commit_msg notes raw_notes has_section
    while IFS= read -r sha; do
        [[ -z "$sha" ]] && continue
        subject=$(git show -s --format=%s "$sha")
        marker=""

        # Extract category from "[Category]" or "[Category]!" format. The
        # space after the bracket is optional, as it was before the marker.
        if [[ "$subject" =~ ^\[([^\]]+)\](!?)[[:space:]]*(.*)$ ]]; then
            commit_cat="${BASH_REMATCH[1]}"
            marker="${BASH_REMATCH[2]}"
            commit_msg="${BASH_REMATCH[3]}"

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

        # The body heading alone marks a commit as breaking; so does the "!".
        if raw_notes=$(breaking_section_from_body "$sha"); then
            has_section=true
        else
            has_section=false
        fi
        if [[ "$marker" == "!" || "$has_section" == "true" ]]; then
            notes=$(printf '%s\n' "$raw_notes" | indent_breaking_notes)
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
