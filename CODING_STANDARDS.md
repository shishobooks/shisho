# Coding Standards

Review-time rules for the code-review Standards reviewer. Apply them to the diff under review. Rules that lint or tests enforce (golangci-lint, ESLint, `scripts/check-emdash.sh`, `pkg/migrations/schema_invariants_test.go`, `pkg/plugins/sdk_sync_test.go`) are not repeated here; a green check settles them.

Read the standards file for each area the diff touches:

- Go backend (`pkg/`, `cmd/`): `docs/agents/standards/backend.md`
- Frontend (`app/`): `docs/agents/standards/frontend.md`

The root and subdirectory `AGENTS.md` files and the topic docs they point to are also review sources: a violation of a rule there is a finding.

## Every diff

- **Tests prove the change.** A bug fix or feature carries a test that fails without the change. Check by reverting the fix locally or by reasoning about the assertion; a test that would pass either way is a finding.
- **New Go tests call `t.Parallel()` first**, unless they touch shared global state (`pkg/config`, the plugin tests listed in `pkg/plugins/AGENTS.md`). Lint does not check this.
- **User-facing changes update `website/docs/`** in the same diff, and only the pages the change affects: a docs edit for an unaffected page is also a finding.
- **Breaking changes carry both markers**: `!` after the category in the title and a `## BREAKING CHANGES` section in the PR body (root `AGENTS.md`).
- **New `config.Config` fields** also land in `shisho.example.yaml`, `website/docs/configuration.md`, and `AdminSettings.tsx`.
- **Instruction-file edits meet the bar** in "Adding to these files" in root `AGENTS.md`: a new line states a rule that outlives the fix, sits in the topic doc its subject belongs to, and carries no bug history or PR numbers. A rule a check could enforce should be a check. Flag statements in an edited instruction file that the diff made wrong.
- **The change matches its neighbors.** A new handler, hook, component, or test reads like the surrounding ones in naming, error handling, and structure. A new pattern needs a reason.
