import { AlertTriangle } from "lucide-react";

interface OldNameAliasWarningProps {
  /** The name when the dialog opened. */
  oldName: string;
  newName: string;
  /** The aliases as they will be saved. */
  aliases: string[];
  /** "person", "genre", and so on. */
  entityLabel: string;
}

/**
 * Warns when a rename drops the old name from the aliases. Files still name
 * the old name, so a Scan for new metadata of their books finds no resource
 * by it and moves them to a separate one with that name, leaving the renamed
 * resource behind (deleted once it has no books). The rename dialogs add the
 * old name as an alias for this reason; this says what removing it does.
 */
export const OldNameAliasWarning = ({
  oldName,
  newName,
  aliases,
  entityLabel,
}: OldNameAliasWarningProps) => {
  const old = oldName.trim();
  const next = newName.trim();
  if (!old || !next || old.toLowerCase() === next.toLowerCase()) return null;
  if (aliases.some((a) => a.trim().toLowerCase() === old.toLowerCase())) {
    return null;
  }
  return (
    <p className="flex gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-400">
      <AlertTriangle
        aria-hidden="true"
        className="mt-px h-3.5 w-3.5 shrink-0"
      />
      <span>
        Without "{old}" as an alias, books whose files still name "{old}" will
        move to a separate {entityLabel} with that name the next time they are
        scanned for new metadata.
      </span>
    </p>
  );
};
