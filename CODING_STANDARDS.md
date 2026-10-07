# Coding Standards

Review-time rules for the code-review Standards reviewer. Apply them to the diff under review. Rules that lint or tests enforce are not repeated here; a green check settles them.

Read the standards file for each area the diff touches:

- Go backend (`pkg/`, `cmd/`): `docs/agents/standards/backend.md`
- Frontend (`app/`): `docs/agents/standards/frontend.md`

The root and subdirectory `AGENTS.md` files and the topic docs they point to are also review sources: a violation of a rule there is a finding.

## Every diff

- **Tests prove the change.** A bug fix or feature carries a test that fails without the change. Check by reverting the fix locally or by reasoning about the assertion; a test that would pass either way is a finding.
- **Root `AGENTS.md` rules apply to every diff**: "Testing" (`t.Parallel()`), "Docs and config must move with behavior", and "Git conventions" (breaking-change markers). Docs edits touch only the pages the change affects, and only with what an operator needs (the inclusion test in `website/AGENTS.md`): an edit to an unaffected page, or implementation detail in `website/docs/`, is a finding.
- **Instruction-file edits meet the bar** in "Adding to these files" in root `AGENTS.md`: cross-cutting how/why or trap guidance only; a rule about one location is a code comment there; no copies of what code or config holds (lists, values, signatures); no bug history; a rule a check could enforce is a check. Flag statements in an edited instruction file that the diff made wrong.
- **The change matches its neighbors.** A new handler, hook, component, or test reads like the surrounding ones in naming, error handling, and structure. A new pattern needs a reason.
