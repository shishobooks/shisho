import { formatCount } from "@/utils/format";

/** What a rename or merge rewrites the sidecars of, in the page's own unit. */
export interface SidecarCount {
  count: number;
  singular: string;
  plural: string;
}

// A rename or merge rewrites the sidecar files of everything that names the
// entity, in a background job. At this many the job takes long enough to
// mention.
const NOTE_THRESHOLD = 100;

interface SidecarRewriteNoteProps {
  /** "Renaming" or "Merging". */
  action: string;
  affected: SidecarCount;
}

/**
 * Says that a big rename or merge rewrites sidecar files in the background,
 * so the save returning at once doesn't read as the work being done.
 */
export const SidecarRewriteNote = ({
  action,
  affected,
}: SidecarRewriteNoteProps) => {
  if (affected.count < NOTE_THRESHOLD) return null;
  return (
    <p className="text-sm text-muted-foreground">
      {action} updates the sidecar files for{" "}
      {formatCount(affected.count, affected.singular, affected.plural)} in the
      background. On a large library that can take a while.
    </p>
  );
};
