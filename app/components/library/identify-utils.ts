export type FieldStatus = "unchanged" | "changed" | "new";

export interface IdentifierEntry {
  type: string;
  value: string;
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
  if (current.length === 0 && incoming.length > 0)
    return { value: incoming, status: "new" };
  if (current.length > 0 && incoming.length === 0)
    return { value: current, status: "unchanged" };
  const key = (id: IdentifierEntry) => `${id.type}:${id.value}`;
  const curKeys = current.map(key).sort();
  const incKeys = incoming.map(key).sort();
  if (
    curKeys.length === incKeys.length &&
    curKeys.every((v, i) => v === incKeys[i])
  ) {
    return { value: current, status: "unchanged" };
  }
  // Merge: keep all current, add new incoming identifiers
  const existingKeys = new Set(current.map(key));
  const newFromIncoming = incoming.filter((id) => !existingKeys.has(key(id)));
  if (newFromIncoming.length === 0) {
    // Incoming is a subset of current — nothing new to add
    return { value: current, status: "unchanged" };
  }
  return { value: [...current, ...newFromIncoming], status: "changed" };
}
