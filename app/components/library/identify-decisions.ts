import type { FieldStatus } from "./identify-utils";

export type FieldScope = "book" | "file";

export interface FieldDecisionInput {
  scope: FieldScope;
  status: FieldStatus;
  isPrimaryFile: boolean;
}

/** Default per-field checkbox state at dialog open.
 *
 * - File-level fields: ON whenever there's something to apply (each file has
 *   its own copy, so applying the plugin's value carries no shared-data risk).
 * - Book-level new: ON (no current value to overwrite).
 * - Book-level changed: ON only on the primary file (avoids non-canonical
 *   files clobbering shared book metadata — the "second-identify" case).
 * - Unchanged: OFF.
 *
 * See spec `docs/superpowers/specs/2026-05-01-identify-flow-design.md`. */
export function defaultDecision({
  scope,
  status,
  isPrimaryFile,
}: FieldDecisionInput): boolean {
  if (status === "unchanged") return false;
  if (scope === "file") return true;
  if (status === "new") return true;
  return isPrimaryFile;
}

/** Combine child decisions into a section-level (or global) state.
 *
 * Returns `"indeterminate"` when some-but-not-all are true, mirroring
 * browser indeterminate semantics. Empty list collapses to `false`. */
export function aggregateDecisions(
  decisions: boolean[],
): boolean | "indeterminate" {
  if (decisions.length === 0) return false;
  const allTrue = decisions.every(Boolean);
  if (allTrue) return true;
  const anyTrue = decisions.some(Boolean);
  return anyTrue ? "indeterminate" : false;
}
