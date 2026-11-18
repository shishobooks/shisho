import { useSidecarImpact } from "@/hooks/queries/books";
import type { SidecarImpactKind } from "@/types";
import { formatCount } from "@/utils/format";

// A rename or merge rewrites the sidecar files of every Book that names the
// entity, in a background job. At this many Books the job takes long enough
// to mention.
const NOTE_THRESHOLD = 100;

interface SidecarRewriteNoteProps {
  /** "Renaming" or "Merging". */
  action: string;
  kind: SidecarImpactKind;
  /** The entity renamed, or a merge's source: the one whose name changes. */
  id: number;
}

/**
 * Says that a big rename or merge rewrites sidecar files in the background,
 * so the save returning at once doesn't read as the work being done. The
 * server counts the distinct Books whose sidecars name the entity whose name
 * changes: the one renamed, or a merge's source.
 */
export const SidecarRewriteNote = ({
  action,
  kind,
  id,
}: SidecarRewriteNoteProps) => {
  const { data } = useSidecarImpact(kind, id);
  // A hint, not content: while the count loads, or if it fails, the dialog
  // just goes without it.
  if (!data || data.book_count < NOTE_THRESHOLD) return null;
  return (
    <p className="text-sm text-muted-foreground">
      {action} updates the sidecar files of{" "}
      {formatCount(data.book_count, "book")} in the background. On a large
      library that can take a while.
    </p>
  );
};
