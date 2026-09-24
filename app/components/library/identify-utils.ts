import {
  SourceIntentPlugin,
  SourceIntentUser,
  type Book,
  type File,
  type SourceIntent,
} from "@/types";

export type FieldStatus = "unchanged" | "changed" | "new";

export interface IdentifierEntry {
  type: string;
  value: string;
}

/** Choose which file to identify when the dialog opens.
 *
 * Decision order, leveraging the existing `file.reviewed` flag computed by
 * `pkg/books/review` against per-library required fields:
 *
 * 1. If some main files are reviewed and others aren't, prefer a non-reviewed
 *    one (`reviewed !== true` covers both `false` and `undefined`).
 * 2. Otherwise the first main file.
 *
 * Returns `undefined` when there are no main files. */
export function pickInitialFile(book: Book): File | undefined {
  const mains = (book.files ?? []).filter((f) => f.file_role === "main");
  if (mains.length === 0) return undefined;
  const hasReviewed = mains.some((f) => f.reviewed === true);
  const hasNonReviewed = mains.some((f) => f.reviewed !== true);
  if (hasReviewed && hasNonReviewed) {
    const nonReviewed = mains.find((f) => f.reviewed !== true);
    if (nonReviewed) return nonReviewed;
  }
  return mains[0];
}

export interface SkippedPlugin {
  plugin_id: string;
  plugin_name?: string;
}

export interface IdentifyEmptyStateInput {
  hasEnricherPlugins: boolean;
  totalPlugins: number;
  skippedPlugins: SkippedPlugin[];
  fileType: string | undefined;
}

export interface IdentifyEmptyStateMessage {
  primary: string;
  secondary?: string;
}

export function computeIdentifyEmptyState(
  input: IdentifyEmptyStateInput,
): IdentifyEmptyStateMessage {
  const fileTypeLabel = input.fileType?.toUpperCase() ?? "this file type";
  const skippedNames = input.skippedPlugins
    .map((p) => p.plugin_name || p.plugin_id)
    .join(", ");

  if (!input.hasEnricherPlugins) {
    return {
      primary:
        "No metadata enricher plugins are installed. Install one from the plugin settings to search for books.",
    };
  }

  if (input.totalPlugins === 0) {
    return {
      primary: "No metadata enricher plugins are enabled for this library.",
    };
  }

  const allSkipped = input.skippedPlugins.length >= input.totalPlugins;

  if (allSkipped) {
    const plural = input.skippedPlugins.length !== 1;
    return {
      primary: `No installed enricher${plural ? "s" : ""} support${plural ? "" : "s"} ${fileTypeLabel} files (${skippedNames}).`,
    };
  }

  if (input.skippedPlugins.length > 0) {
    const plural = input.skippedPlugins.length !== 1;
    return {
      primary: "Try a different search query.",
      secondary: plural
        ? `${skippedNames} were skipped because they don't support ${fileTypeLabel} files.`
        : `${skippedNames} was skipped because it doesn't support ${fileTypeLabel} files.`,
    };
  }

  return { primary: "Try a different search query." };
}

/** Collapse duplicate types, last wins. The DB invariant is one identifier
 * per type per file, so a misbehaving plugin's duplicate-type proposal is
 * read the way the store would persist it. */
function dedupeIdentifiersByType(
  entries: IdentifierEntry[],
): IdentifierEntry[] {
  const deduped: IdentifierEntry[] = [];
  const indexByType = new Map<string, number>();
  for (const entry of entries) {
    const existingIdx = indexByType.get(entry.type);
    if (existingIdx === undefined) {
      indexByType.set(entry.type, deduped.length);
      deduped.push(entry);
    } else {
      deduped[existingIdx] = entry;
    }
  }
  return deduped;
}

const identifierKey = (id: IdentifierEntry) => `${id.type}|${id.value.trim()}`;

export function resolveIdentifiers(
  current: IdentifierEntry[],
  incoming: IdentifierEntry[],
): { value: IdentifierEntry[]; status: FieldStatus } {
  const dedupedIncoming = dedupeIdentifiersByType(incoming);

  if (current.length === 0 && dedupedIncoming.length === 0) {
    return { value: [], status: "unchanged" };
  }
  if (current.length === 0) {
    return { value: dedupedIncoming, status: "new" };
  }
  if (dedupedIncoming.length === 0) {
    return { value: current, status: "unchanged" };
  }

  // Overwrite: use incoming identifiers directly (replacing current).
  return {
    value: dedupedIncoming,
    status: identifierSetsEqual(current, dedupedIncoming)
      ? "unchanged"
      : "changed",
  };
}

/** Set equality on (type, trimmed value), the same comparison Identify uses
 * for status and intent. */
export function identifierSetsEqual(
  a: IdentifierEntry[],
  b: IdentifierEntry[],
): boolean {
  const aSet = new Set(a.map(identifierKey));
  const bSet = new Set(b.map(identifierKey));
  return aSet.size === bSet.size && [...aSet].every((k) => bSet.has(k));
}

/**
 * Per-entry Identify source intent for one identifier (ADR 0006): "plugin"
 * when an entry of the same type and trimmed value is in the Plugin
 * Proposal, "user" otherwise. The server keeps a retained entry's stored
 * source regardless, so this only decides new or replaced entries.
 */
export function identifierEntryIntent(
  entry: IdentifierEntry,
  proposal: IdentifierEntry[],
): SourceIntent {
  const proposed = new Set(
    dedupeIdentifiersByType(proposal).map(identifierKey),
  );
  return proposed.has(identifierKey(entry))
    ? SourceIntentPlugin
    : SourceIntentUser;
}

/**
 * Field-level Identify source intent for the identifier collection: "plugin"
 * only when the final collection equals the Plugin Proposal as a set of
 * (type, trimmed value). Any other nonempty composition, and a clear, is
 * "user"; the server nulls the aggregate source on a clear either way.
 */
export function identifierCollectionIntent(
  finalValue: IdentifierEntry[],
  proposal: IdentifierEntry[],
): SourceIntent {
  if (finalValue.length === 0) return SourceIntentUser;
  return identifierSetsEqual(finalValue, dedupeIdentifiersByType(proposal))
    ? SourceIntentPlugin
    : SourceIntentUser;
}

/**
 * Identify source intent for a scalar (ADR 0006): "plugin" when the final
 * value equals the Plugin Proposal, "user" otherwise. Equality is a trimmed
 * raw comparison, so editing and then restoring the proposal is "plugin".
 * Whether the value is a no-op against stored metadata is decided by the
 * server, which holds the canonical stored state.
 */
export function scalarSourceIntent(
  finalValue: string,
  proposal: string | undefined | null,
): SourceIntent {
  return finalValue.trim() === (proposal ?? "").trim()
    ? SourceIntentPlugin
    : SourceIntentUser;
}

/**
 * scalarSourceIntent for a nullable boolean, where `null` is an Explicit
 * Clear and `undefined` means the plugin proposed nothing. Neither can match
 * a proposal, which keeps `false` distinct from absence.
 */
export function booleanSourceIntent(
  finalValue: boolean | null,
  proposal: boolean | undefined | null,
): SourceIntent {
  return finalValue !== null && finalValue === proposal
    ? SourceIntentPlugin
    : SourceIntentUser;
}
