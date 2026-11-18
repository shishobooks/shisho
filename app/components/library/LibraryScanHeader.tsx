import { Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useGettingStartedPanel } from "@/contexts/GettingStarted/useGettingStartedPanel";
import { useGettingStarted } from "@/hooks/queries/gettingStarted";
import type { LibraryScan } from "@/hooks/useLibraryScan";
import { formatCount } from "@/utils/format";

interface LibraryScanHeaderProps {
  scan: LibraryScan;
}

const scanDetail = ({
  progress,
  scanForNewMetadata,
  everyLibrary,
}: LibraryScan): string | null => {
  if (everyLibrary) return null;
  if (!progress) return "Looking for files…";
  // Until the first file is done, the scan is still finding files.
  if (scanForNewMetadata && progress.files_scanned > 0) {
    return `${progress.files_scanned.toLocaleString()} of ${formatCount(progress.files_found, "file")} scanned. Manual edits are kept.`;
  }
  if (scanForNewMetadata) {
    return `${formatCount(progress.files_found, "file")} found`;
  }
  return `${formatCount(progress.files_found, "file")} found · ${formatCount(progress.books_imported, "book")} imported`;
};

/**
 * Says on the library page itself that a scan is running and how far it has
 * got, at every width, since the nav's resync icon is hidden on phones.
 */
export const LibraryScanHeader = ({ scan }: LibraryScanHeaderProps) => {
  const title = scan.everyLibrary
    ? scan.scanForNewMetadata
      ? "Scanning your libraries for new metadata"
      : "Scanning your libraries"
    : scan.scanForNewMetadata
      ? "Scanning for new metadata"
      : "Scanning this library";
  const detail = scanDetail(scan);

  // While books arrive, point to what's left to set up, as long as the
  // Getting Started list is on and has a step left.
  const { data: gettingStarted } = useGettingStarted();
  const { setOpen: openGettingStarted } = useGettingStartedPanel();
  const stepsLeft =
    gettingStarted?.enabled === true &&
    !gettingStarted.dismissed &&
    gettingStarted.steps.some((s) => s.applies && !s.done && !s.not_for_me);

  return (
    <section
      aria-label={title}
      className="mb-6 flex items-center gap-3 rounded-md border border-border bg-muted/40 px-4 py-3"
      role="status"
    >
      <Loader2
        aria-hidden="true"
        className="h-5 w-5 shrink-0 animate-spin text-primary"
      />
      <div className="min-w-0">
        <p className="text-sm font-medium text-foreground">{title}</p>
        {detail && <p className="text-sm text-muted-foreground">{detail}</p>}
        {stepsLeft && (
          <p className="text-sm text-muted-foreground">
            While it runs, see{" "}
            <Button
              className="h-auto p-0 text-sm"
              onClick={() => openGettingStarted(true)}
              variant="link"
            >
              Getting Started
            </Button>{" "}
            for what to set up next.
          </p>
        )}
      </div>
    </section>
  );
};
