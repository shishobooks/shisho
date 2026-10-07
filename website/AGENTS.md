# Documentation Site

## Mission and Audience

Shisho's documentation helps readers decide whether Shisho fits their library, deploy it safely, operate it confidently, and use its supported extension points. Write for these audiences in priority order:

1. Prospective deployers evaluating Shisho
2. Operators and administrators responsible for an installation
3. Regular users managing and reading books
4. Readers configuring integrations
5. Plugin developers

Lead with the reader's task and the consequence of each choice. The docs are a product manual, not a record of the codebase.

## Information Architecture

`website/sidebars.ts` alone sets navigation and order; its comments give the ordering rules and the legacy files left out on purpose. Place every new page where its intended reader would seek it, never at the bottom by default. Optional or niche does not mean last. If a page changes a category's purpose or audience, reconsider the architecture instead of forcing it into the nearest directory.

## Editorial Policy

### Inclusion Test

Document a fact only when all five statements are true:

1. It is shipped in the version being documented.
2. It is relevant to a user, operator, integration owner, or plugin developer.
3. It teaches something beyond obvious labels in the interface.
4. It has one canonical page that owns the fact.
5. It describes a stable behavior or public contract that readers can rely on.

If one statement is false, leave the detail out or wait until the feature and contract are ready.

### Exclusions

Do not publish:

- Roadmaps, planned features, speculation, or design proposals
- Internal architecture that does not change a supported user task
- Database schemas, package layout, migration details, or test implementation
- Unsupported endpoints, private APIs, or incidental implementation behavior
- Changelog-style narration of how a feature evolved
- Links to any `AGENTS.md` as user documentation

Link to an external technical source only when it is part of a supported contract or necessary context.

### Pages, Sections, and Categories

Create a page when a task has a distinct reader goal, enough durable content to stand alone, and a useful URL to link from support or the interface. Add a section when the material supports the same goal and audience as its parent page. Create or change a category only when several pages share a clear audience and purpose that the current architecture cannot express. Do not create landing pages that only repeat the sidebar.

Split mixed-audience content. Put operator setup and security decisions in Administration, daily workflows in Using Shisho, integration setup in Integrations, and plugin contracts in Developer. Cross-link at the decision point rather than copying instructions.

### Technical Detail

Include enough technical detail for a reader to make a decision, verify state, or recover from failure. Prefer exact supported values, paths, UI labels, and observable outcomes. Omit implementation mechanics that do not alter those actions.

Do not infer behavior from a type name, route, or implementation alone. Verify user-facing claims against the interface, configuration reference, tests, and implementation as needed. Do not invent commands, recovery behavior, compatibility, defaults, or guarantees.

### Canonical Ownership

Every fact has one canonical owner. Other pages summarize only the context needed to route the reader, then link to the owner. The homepage evaluates and routes; it is not the canonical deployment guide. Troubleshooting diagnoses and routes; it does not become a second configuration reference.

Before adding content, search current docs for the concept and choose the owner. When moving content, update inbound links and remove stale copies. Repeated text that can drift is a defect.

### UI Writing

Use the exact current UI label and path, with bold text for controls and navigation. Describe navigation with `>` separators, for example **Settings > Libraries**. State required permissions when access may differ by role. Do not claim an action is automatic when it requires a setting, plugin, job, or user choice.

### Destructive and Security-Sensitive Actions

Place a warning immediately before a destructive or security-sensitive step. State:

- What data, files, credentials, or access can change
- Whether the action is reversible
- What the reader should back up or verify first
- The safest alternative when one exists

Never bury file moves, deletion, permission expansion, secret rotation, remote access, or cache removal in a general note. Distinguish database deletion from files on disk. Distinguish read access from write access. Do not print real secrets in examples.

### Troubleshooting Format

Troubleshooting entries use this order:

1. **Symptom**: what the reader observes
2. **Likely cause**: the smallest set of supported explanations
3. **Verify**: a safe observation that separates those explanations
4. **Fix**: the least destructive supported correction, with a link to the canonical page

Do not start with broad cleanup or deletion. Preserve evidence such as job logs and current-session server logs until the cause is understood.

### Titles, Links, and URLs

Page titles use Title Case. Capitalize principal words and lowercase articles, coordinating conjunctions, and short prepositions. Preserve branded casing such as eReader, iPhone, and macOS.

When changing a title, grep current docs for its old display text and update references. Preserve an established URL unless a redirect is deliberately added and verified. Prefer relative links between docs. Use fragment links only after checking the rendered heading ID. Broken links fail the Docusaurus build.

## Versioned Docs

Current work belongs in `website/docs/` and `website/sidebars.ts`. `website/versioned_docs/` and `website/versioned_sidebars/` are frozen snapshots that `scripts/release.sh` takes at each release; never edit them to backport current behavior. Correct a versioned page only when it was factually wrong for that release, as an explicit maintenance task.

### Linking to Unreleased Pages from Site Pages

`/docs/<id>` serves the latest released snapshot, and the build fails when a page in `website/src/pages/` links to a doc that snapshot does not contain yet. A doc added since the last release exists only at `/docs/unreleased/<id>` until the next release. Links inside `website/docs/` are unaffected because they resolve within the same version.

Do not hardcode `/docs/unreleased/<id>` in a site page, and do not add the page to `versioned_docs/` to satisfy the check. Either wait for the next release and then link `/docs/<id>`, or resolve the path at runtime with a small hook built on `useVersions()` and `useLatestVersion()` from `@docusaurus/plugin-content-docs/client`, and open a follow-up issue to delete it after the release.

## Verifying a Docs Change

Run `cd website && pnpm build` (it catches broken links) and, when theme or navigation behavior changes, `mise e2e:docs`. Deployment belongs to `.github/workflows/docs.yml`; never run `docusaurus deploy` by hand.

## Theme Gotchas

- Use `background-image`, not the `background` shorthand, for gradients clipped to text. The production CSS pipeline adds Display P3 overrides, and an overriding shorthand resets `background-clip` while the text stays transparent, so the bug shows only in production builds.
- The root `.gitignore` ignores macOS `Icon` files, which also matches swizzled `Icon/` directories. Add an exception there before creating another one, or Git silently ignores it.
