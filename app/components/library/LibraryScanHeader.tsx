import { Loader2 } from "lucide-react";

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
      </div>
    </section>
  );
};
