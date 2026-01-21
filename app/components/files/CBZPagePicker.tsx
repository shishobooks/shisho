import { useMemo, useState } from "react";

import CBZPagePreview from "@/components/files/CBZPagePreview";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/libraries/utils";

export interface CBZPagePickerProps {
  fileId: number;
  pageCount: number;
  currentPage: number | null;
  onSelect: (page: number) => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Dialog for selecting a page from a CBZ file.
 * Shows a grid of page thumbnails with lazy loading of pages in batches of 10.
 */
const CBZPagePicker = ({
  fileId,
  pageCount,
  currentPage,
  onSelect,
  open,
  onOpenChange,
}: CBZPagePickerProps) => {
  // Calculate initial range: current page +/-10, or 0-9 if no current page
  const initialRange = useMemo(() => {
    if (currentPage != null) {
      const start = Math.max(0, currentPage - 10);
      const end = Math.min(pageCount - 1, currentPage + 10);
      return { start, end };
    }
    return { start: 0, end: Math.min(9, pageCount - 1) };
  }, [currentPage, pageCount]);

  const [visibleRange, setVisibleRange] = useState(initialRange);

  // Reset visible range when dialog opens or currentPage changes
  const handleOpenChange = (newOpen: boolean) => {
    if (newOpen) {
      // Reset to initial range when opening
      setVisibleRange(initialRange);
    }
    onOpenChange(newOpen);
  };

  // Load previous 10 pages
  const handleLoadPrevious = () => {
    setVisibleRange((prev) => ({
      ...prev,
      start: Math.max(0, prev.start - 10),
    }));
  };

  // Load next 10 pages
  const handleLoadNext = () => {
    setVisibleRange((prev) => ({
      ...prev,
      end: Math.min(pageCount - 1, prev.end + 10),
    }));
  };

  // Handle page selection
  const handleSelectPage = (page: number) => {
    onSelect(page);
    onOpenChange(false);
  };

  // Generate array of page numbers to display
  const pages = useMemo(() => {
    const result: number[] = [];
    for (let i = visibleRange.start; i <= visibleRange.end; i++) {
      result.push(i);
    }
    return result;
  }, [visibleRange]);

  const canLoadPrevious = visibleRange.start > 0;
  const canLoadNext = visibleRange.end < pageCount - 1;

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent className="max-w-2xl max-h-[80vh] overflow-y-auto overflow-x-hidden">
        <DialogHeader className="pr-8">
          <DialogTitle>Select Page</DialogTitle>
        </DialogHeader>

        <div className="flex flex-col gap-4">
          {/* Load previous button */}
          {canLoadPrevious && (
            <Button
              className="w-full"
              onClick={handleLoadPrevious}
              type="button"
              variant="outline"
            >
              Load previous 10
            </Button>
          )}

          {/* Page grid */}
          <div className="grid grid-cols-4 gap-3 sm:grid-cols-5">
            {pages.map((page) => (
              <CBZPagePreview
                fileId={fileId}
                key={page}
                page={page}
                previewSize={400}
                thumbnailSize={100}
              >
                <div
                  className={cn(
                    "relative cursor-pointer rounded-md overflow-hidden border border-border bg-muted",
                    page === currentPage &&
                      "ring-2 ring-primary ring-offset-2 ring-offset-background",
                  )}
                  onClick={() => handleSelectPage(page)}
                  style={{ height: "100px" }}
                >
                  <img
                    alt={`Page ${page + 1}`}
                    className="h-full w-full object-contain"
                    src={`/api/books/files/${fileId}/page/${page}`}
                  />
                  <div className="absolute bottom-0 left-0 right-0 bg-black/60 text-white text-xs text-center py-0.5">
                    {page + 1}
                  </div>
                </div>
              </CBZPagePreview>
            ))}
          </div>

          {/* Load next button */}
          {canLoadNext && (
            <Button
              className="w-full"
              onClick={handleLoadNext}
              type="button"
              variant="outline"
            >
              Load next 10
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
};

export default CBZPagePicker;
