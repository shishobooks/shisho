import {
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ScrollArea, ScrollBar } from "@/components/ui/scroll-area";
import { cn } from "@/libraries/utils";

export interface CBZPagePickerProps {
  fileId: number;
  pageCount: number;
  currentPage: number | null;
  onSelect: (page: number) => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title?: string;
}

/**
 * Dialog for selecting a page from a CBZ file.
 * Shows a large preview of the focused page with a scrollable thumbnail strip below.
 */
const CBZPagePicker = ({
  fileId,
  pageCount,
  currentPage,
  onSelect,
  open,
  onOpenChange,
  title = "Select Page",
}: CBZPagePickerProps) => {
  // Track the currently focused/previewed page (not necessarily the selected one)
  const [focusedPage, setFocusedPage] = useState<number>(currentPage ?? 0);
  // Track the last loaded page to avoid flashing during transitions
  const [loadedPage, setLoadedPage] = useState<number | null>(null);
  const thumbnailStripRef = useRef<HTMLDivElement>(null);
  const thumbnailRefs = useRef<Map<number, HTMLButtonElement>>(new Map());

  // Reset state when dialog opens
  const handleOpenChange = (newOpen: boolean) => {
    if (newOpen) {
      setFocusedPage(currentPage ?? 0);
      setLoadedPage(null);
    }
    onOpenChange(newOpen);
  };

  // Navigate to previous/next page
  const goToPrevious = useCallback(() => {
    setFocusedPage((prev) => Math.max(0, prev - 1));
  }, []);

  const goToNext = useCallback(() => {
    setFocusedPage((prev) => Math.min(pageCount - 1, prev + 1));
  }, [pageCount]);

  // Jump 10 pages
  const jumpBackward = useCallback(() => {
    setFocusedPage((prev) => Math.max(0, prev - 10));
  }, []);

  const jumpForward = useCallback(() => {
    setFocusedPage((prev) => Math.min(pageCount - 1, prev + 10));
  }, [pageCount]);

  // Handle thumbnail click - focus on that page
  const handleThumbnailClick = (page: number) => {
    setFocusedPage(page);
  };

  // Handle confirming the selection
  const handleConfirmSelection = useCallback(() => {
    onSelect(focusedPage);
    onOpenChange(false);
  }, [focusedPage, onOpenChange, onSelect]);

  // Keyboard navigation (matches CBZReader shortcuts)
  useEffect(() => {
    if (!open) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      switch (e.key) {
        case "ArrowLeft":
        case "a":
        case "A":
          e.preventDefault();
          goToPrevious();
          break;
        case "ArrowRight":
        case "d":
        case "D":
          e.preventDefault();
          goToNext();
          break;
        case "Enter":
          e.preventDefault();
          handleConfirmSelection();
          break;
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [open, goToPrevious, goToNext, handleConfirmSelection]);

  // Scroll focused thumbnail into view
  useEffect(() => {
    if (!open) return;
    const thumbnail = thumbnailRefs.current.get(focusedPage);
    if (thumbnail) {
      thumbnail.scrollIntoView({
        behavior: "smooth",
        block: "nearest",
        inline: "center",
      });
    }
  }, [focusedPage, open]);

  // Generate visible thumbnail range (focused page ± 20)
  const visibleThumbnails = useMemo(() => {
    const start = Math.max(0, focusedPage - 20);
    const end = Math.min(pageCount - 1, focusedPage + 20);
    const result: number[] = [];
    for (let i = start; i <= end; i++) {
      result.push(i);
    }
    return result;
  }, [focusedPage, pageCount]);

  const canGoPrevious = focusedPage > 0;
  const canGoNext = focusedPage < pageCount - 1;

  // Show loading spinner only on initial load, not during navigation
  const isInitialLoad = loadedPage === null;

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent className="max-w-4xl w-[95vw] h-[90vh] flex flex-col gap-0 p-0 overflow-hidden">
        <DialogHeader className="px-6 py-4 pr-14 border-b border-border/50 shrink-0">
          <DialogTitle className="flex items-center justify-between">
            <span>{title}</span>
            <span className="text-sm font-normal text-muted-foreground tabular-nums">
              Page {focusedPage + 1} of {pageCount}
            </span>
          </DialogTitle>
        </DialogHeader>

        {/* Main preview area - fixed height calculation */}
        <div className="relative flex-1 min-h-0 bg-black/95 flex items-center justify-center overflow-hidden">
          {/* Navigation buttons - large touch targets */}
          <button
            className={cn(
              "absolute left-2 top-1/2 -translate-y-1/2 z-10",
              "w-12 h-24 rounded-lg bg-black/40 hover:bg-black/60 backdrop-blur-sm",
              "flex items-center justify-center transition-all duration-200",
              "text-white/70 hover:text-white",
              "disabled:opacity-20 disabled:cursor-not-allowed disabled:hover:bg-black/40",
            )}
            disabled={!canGoPrevious}
            onClick={goToPrevious}
            type="button"
          >
            <ChevronLeft className="w-8 h-8" />
          </button>

          <button
            className={cn(
              "absolute right-2 top-1/2 -translate-y-1/2 z-10",
              "w-12 h-24 rounded-lg bg-black/40 hover:bg-black/60 backdrop-blur-sm",
              "flex items-center justify-center transition-all duration-200",
              "text-white/70 hover:text-white",
              "disabled:opacity-20 disabled:cursor-not-allowed disabled:hover:bg-black/40",
            )}
            disabled={!canGoNext}
            onClick={goToNext}
            type="button"
          >
            <ChevronRight className="w-8 h-8" />
          </button>

          {/* Main image container */}
          <div className="w-full h-full flex items-center justify-center p-4 px-16">
            {isInitialLoad && (
              <div className="absolute inset-0 flex items-center justify-center z-10">
                <div className="w-8 h-8 border-2 border-white/20 border-t-white/80 rounded-full animate-spin" />
              </div>
            )}
            <img
              alt={`Page ${focusedPage + 1}`}
              className="max-h-full max-w-full object-contain rounded shadow-2xl"
              onLoad={() => setLoadedPage(focusedPage)}
              src={`/api/books/files/${fileId}/page/${focusedPage}`}
            />
          </div>
        </div>

        {/* Thumbnail strip */}
        <div className="shrink-0 border-t border-border/50 bg-card">
          {/* Jump buttons and scroll area */}
          <div className="flex items-center gap-2 px-3 py-3">
            <Button
              className="shrink-0 h-9 w-9 p-0"
              disabled={focusedPage < 10}
              onClick={jumpBackward}
              size="sm"
              title="Jump back 10 pages"
              variant="ghost"
            >
              <ChevronsLeft className="w-4 h-4" />
            </Button>

            <ScrollArea className="flex-1">
              <div className="flex gap-2 py-1 px-1" ref={thumbnailStripRef}>
                {visibleThumbnails.map((page) => (
                  <button
                    className={cn(
                      "relative shrink-0 rounded overflow-hidden transition-all duration-150",
                      "border-2 bg-muted",
                      "hover:border-primary/50 hover:scale-105",
                      "focus:outline-none focus-visible:ring-2 focus-visible:ring-primary",
                      page === focusedPage
                        ? "border-primary ring-2 ring-primary/30 scale-105"
                        : page === currentPage
                          ? "border-amber-500/70"
                          : "border-transparent",
                    )}
                    key={page}
                    onClick={() => handleThumbnailClick(page)}
                    ref={(el) => {
                      if (el) {
                        thumbnailRefs.current.set(page, el);
                      } else {
                        thumbnailRefs.current.delete(page);
                      }
                    }}
                    style={{ width: "72px", height: "96px" }}
                    type="button"
                  >
                    <img
                      alt={`Page ${page + 1}`}
                      className="w-full h-full object-contain"
                      loading="lazy"
                      src={`/api/books/files/${fileId}/page/${page}`}
                    />
                    <div
                      className={cn(
                        "absolute bottom-0 left-0 right-0 text-xs text-center py-0.5 font-medium tabular-nums",
                        page === focusedPage
                          ? "bg-primary text-primary-foreground"
                          : "bg-black/70 text-white",
                      )}
                    >
                      {page + 1}
                    </div>
                  </button>
                ))}
              </div>
              <ScrollBar orientation="horizontal" />
            </ScrollArea>

            <Button
              className="shrink-0 h-9 w-9 p-0"
              disabled={focusedPage >= pageCount - 10}
              onClick={jumpForward}
              size="sm"
              title="Jump forward 10 pages"
              variant="ghost"
            >
              <ChevronsRight className="w-4 h-4" />
            </Button>
          </div>

          {/* Action bar */}
          <div className="flex items-center justify-between gap-4 px-4 py-3 border-t border-border/50 bg-muted/30">
            <p className="text-sm text-muted-foreground">
              <kbd className="px-1.5 py-0.5 rounded bg-muted border text-xs">
                A
              </kbd>{" "}
              <kbd className="px-1.5 py-0.5 rounded bg-muted border text-xs">
                D
              </kbd>{" "}
              or{" "}
              <kbd className="px-1.5 py-0.5 rounded bg-muted border text-xs">
                ←
              </kbd>{" "}
              <kbd className="px-1.5 py-0.5 rounded bg-muted border text-xs">
                →
              </kbd>{" "}
              to navigate,{" "}
              <kbd className="px-1.5 py-0.5 rounded bg-muted border text-xs">
                Enter
              </kbd>{" "}
              to select
            </p>
            <div className="flex gap-2">
              <Button onClick={() => onOpenChange(false)} variant="outline">
                Cancel
              </Button>
              <Button onClick={handleConfirmSelection}>
                Select Page {focusedPage + 1}
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
};

export default CBZPagePicker;
