import {
  DataSourceFileMetadataPriority,
  DataSourcePluginPrefix,
} from "@/types";

import type { FieldStatus } from "./identify-utils";

export type FieldScope = "book" | "file";

export interface FieldDecisionInput {
  scope: FieldScope;
  status: FieldStatus;
  /** The data source of the field's current value (e.g. "manual", "filepath").
   *  Used for book-level changed fields to decide whether the plugin value
   *  should default ON (low-priority source) or OFF (high-priority source). */
  fieldSource: string | undefined;
}

/** Source priority lookup matching the backend's `GetDataSourcePriority`.
 *  Sources at or above `DataSourceFileMetadataPriority` (3) are considered
 *  low-priority and the plugin's value should default ON. */
const SOURCE_PRIORITY: Record<string, number> = {
  manual: 0,
  sidecar: 1,
  plugin: 2,
  file_metadata: 3,
  existing_cover: 3,
  epub_metadata: 3,
  cbz_metadata: 3,
  m4b_metadata: 3,
  pdf_metadata: 3,
  filepath: 4,
};

function getSourcePriority(source: string | undefined): number {
  if (source == null) return DataSourceFileMetadataPriority;
  const p = SOURCE_PRIORITY[source];
  if (p != null) return p;
  if (source.startsWith(DataSourcePluginPrefix)) return 2;
  // Unknown source — treat as low-priority so the plugin value defaults ON.
  return DataSourceFileMetadataPriority;
}

/** Whether a field's current source is low-priority (file metadata or filepath),
 *  meaning the plugin's value should default ON for book-level changed fields. */
function isLowPrioritySource(source: string | undefined): boolean {
  return getSourcePriority(source) >= DataSourceFileMetadataPriority;
}

/** Default per-field checkbox state at dialog open.
 *
 * - File-level fields: ON whenever there's something to apply (each file has
 *   its own copy, so applying the plugin's value carries no shared-data risk).
 * - Book-level new: ON (no current value to overwrite).
 * - Book-level changed: ON when the field's current value came from a
 *   low-priority source (filepath or file metadata); OFF when it came from
 *   a high-priority source (manual, sidecar, or plugin).
 * - Unchanged: OFF.
 *
 */
export function defaultDecision({
  scope,
  status,
  fieldSource,
}: FieldDecisionInput): boolean {
  if (status === "unchanged") return false;
  if (scope === "file") return true;
  if (status === "new") return true;
  // Book-level changed: check source priority.
  return isLowPrioritySource(fieldSource);
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
