import { useEffect } from "react";
import { toast } from "sonner";

import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { useBooks } from "@/hooks/queries/books";
import { scanForNewMetadataPayload, useCreateJob } from "@/hooks/queries/jobs";
import { toastRequestError } from "@/libraries/api";
import { formatCount } from "@/utils/format";

interface EnrichExistingBooksPromptProps {
  /**
   * The name of the metadata enricher just installed, or null when there is
   * nothing to offer.
   */
  pluginName: string | null;
  onClose: () => void;
}

/**
 * After a metadata enricher is installed, offers the library-wide Scan for
 * new metadata, since a normal scan skips Books whose files haven't changed
 * and the enricher would never reach them. It shows only when Books exist.
 * Parents mount it only for a role that can start a scan (Jobs Read and Jobs
 * Write, as the resync button needs).
 */
export const EnrichExistingBooksPrompt = ({
  pluginName,
  onClose,
}: EnrichExistingBooksPromptProps) => {
  // Counted afresh for each install, so the number is current.
  const countQuery = useBooks(
    { limit: 1 },
    { enabled: pluginName !== null, staleTime: 0 },
  );
  const createJob = useCreateJob();

  const counted = countQuery.isSuccess && !countQuery.isFetching;
  const bookCount = countQuery.data?.total ?? 0;
  const open = pluginName !== null && counted && bookCount > 0;

  // With no Books to enrich, the offer lapses, so Books a later scan brings
  // in while the page is open don't raise it.
  useEffect(() => {
    if (pluginName !== null && counted && bookCount === 0) onClose();
  }, [pluginName, counted, bookCount, onClose]);

  const startScan = () => {
    createJob.mutate(
      { payload: scanForNewMetadataPayload() },
      {
        onError: (error) => toastRequestError(error, "Failed to start scan"),
        onSuccess: () => {
          toast.success("Scanning your libraries for new metadata");
          onClose();
        },
      },
    );
  };

  return (
    <ConfirmDialog
      cancelLabel="Not now"
      confirmLabel="Enrich books"
      description={`${pluginName ?? "The plugin"} only runs on books as they're scanned, and scans skip books whose files haven't changed. Scanning for new metadata runs your metadata plugins on every book you already have, replacing details that came from the files themselves. It takes a while and calls the plugins' services for every book. Manual edits and values you chose in Identify are kept.`}
      isPending={createJob.isPending}
      onConfirm={startScan}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      open={open}
      title={`Enrich your ${formatCount(bookCount, "existing book")}?`}
      variant="default"
    />
  );
};
