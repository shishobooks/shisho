import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { DownloadFailure } from "@/hooks/useFileDownload";

interface DownloadErrorDialogProps {
  failure: DownloadFailure | null;
  onClose: () => void;
  /**
   * Downloads the file as it is on disk. Absent on a Share Link page, whose
   * recipient cannot have the unmodified file.
   */
  onDownloadOriginal?: (failure: DownloadFailure) => void;
}

/** What `useFileDownload` reports when a download fails to generate. */
const DownloadErrorDialog = ({
  failure,
  onClose,
  onDownloadOriginal,
}: DownloadErrorDialogProps) => (
  <Dialog
    onOpenChange={(open) => {
      if (!open) onClose();
    }}
    open={!!failure}
  >
    <DialogContent className="max-w-md">
      <DialogHeader>
        <DialogTitle>Download Failed</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <DialogDescription>{failure?.message}</DialogDescription>
        {/* The way out: the file as it is on disk, which skips writing
            Shisho's metadata into it and so cannot fail the same way. */}
        {failure && onDownloadOriginal && (
          <p className="mt-3 text-sm text-muted-foreground">
            You can still download the unmodified file, as it is on disk,
            without the metadata Shisho writes into downloads.
          </p>
        )}
      </DialogBody>
      <DialogFooter>
        <Button onClick={onClose} size="sm" variant="outline">
          Cancel
        </Button>
        {failure && onDownloadOriginal && (
          <Button onClick={() => onDownloadOriginal(failure)} size="sm">
            Download unmodified file
          </Button>
        )}
      </DialogFooter>
    </DialogContent>
  </Dialog>
);

export default DownloadErrorDialog;
