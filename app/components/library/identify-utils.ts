import type { Book, File } from "@/types";

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
 * 2. Otherwise prefer the book's primary file if set.
 * 3. Otherwise the first main file.
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
  if (book.primary_file_id != null) {
    const primary = mains.find((f) => f.id === book.primary_file_id);
    if (primary) return primary;
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

export function resolveIdentifiers(
  current: IdentifierEntry[],
  incoming: IdentifierEntry[],
): { value: IdentifierEntry[]; status: FieldStatus } {
  // Dedupe incoming by type (last-wins) so a misbehaving plugin can't propagate
  // a duplicate-type set forward. The DB invariant is one identifier per type
  // per file; "incoming wins on conflict" extends naturally to "the last
  // incoming entry wins" within the same payload.
  const dedupedIncoming: IdentifierEntry[] = [];
  const incomingByType = new Map<string, number>();
  for (const entry of incoming) {
    const existingIdx = incomingByType.get(entry.type);
    if (existingIdx === undefined) {
      incomingByType.set(entry.type, dedupedIncoming.length);
      dedupedIncoming.push(entry);
    } else {
      dedupedIncoming[existingIdx] = entry;
    }
  }

  if (current.length === 0 && dedupedIncoming.length === 0) {
    return { value: [], status: "unchanged" };
  }
  if (current.length === 0) {
    return { value: dedupedIncoming, status: "new" };
  }
  if (dedupedIncoming.length === 0) {
    return { value: current, status: "unchanged" };
  }

  // Merge: keep current's order; for each current entry, replace value with
  // incoming's value if the type matches. Append new types from incoming
  // (in incoming order) at the end.
  const incomingMap = new Map(dedupedIncoming.map((id) => [id.type, id.value]));
  let changed = false;
  const merged: IdentifierEntry[] = current.map((id) => {
    const incomingValue = incomingMap.get(id.type);
    if (incomingValue !== undefined && incomingValue !== id.value) {
      changed = true;
      return { type: id.type, value: incomingValue };
    }
    return id;
  });
  const currentTypes = new Set(current.map((id) => id.type));
  for (const entry of dedupedIncoming) {
    if (!currentTypes.has(entry.type)) {
      merged.push(entry);
      changed = true;
    }
  }

  return { value: merged, status: changed ? "changed" : "unchanged" };
}
