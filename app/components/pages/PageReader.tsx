import { ChevronLeft, ChevronRight, Settings } from "lucide-react";
import React, {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useNavigate, useSearchParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { useFileChapters } from "@/hooks/queries/chapters";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { useAutoHideChrome } from "@/hooks/useAutoHideChrome";
import { usePageTitle } from "@/hooks/usePageTitle";
import { toastRequestError } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import type { Chapter, UserSettingsPayload } from "@/types";

// Flatten chapters for progress bar (CBZ/PDF chapters don't nest)
const flattenChapters = (chapters: Chapter[]): Chapter[] => {
  const result: Chapter[] = [];
  for (const ch of chapters) {
    if (ch.start_page != null) {
      result.push(ch);
    }
    if (ch.children) {
      result.push(...flattenChapters(ch.children.filter(Boolean) as Chapter[]));
    }
  }
  return result;
};

interface PageReaderProps {
  fileId: number;
  bookId: number;
  libraryId: string;
  totalPages: number;
  getPageUrl: (pageNum: number) => string;
  title?: string;
}

export default function PageReader({
  fileId,
  bookId,
  libraryId,
  totalPages,
  getPageUrl,
  title,
}: PageReaderProps) {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  // Parse page from URL, default to 0
  const urlPage = parseInt(searchParams.get("page") || "0", 10);
  const [currentPage, setCurrentPage] = useState(isNaN(urlPage) ? 0 : urlPage);

  // Image loading state
  const [imageLoading, setImageLoading] = useState(true);
  const currentPageUrl = getPageUrl(currentPage);
  const imgRef = useRef<HTMLImageElement>(null);
  const mainRef = useRef<HTMLElement>(null);

  // Reset scroll position on page change (layout effect to avoid flash)
  useLayoutEffect(() => {
    mainRef.current?.scrollTo(0, 0);
  }, [currentPage]);

  // Reset loading state when the page URL changes
  useEffect(() => {
    // If the image is already cached by the browser, it may be complete immediately
    if (imgRef.current?.complete && imgRef.current.naturalWidth > 0) {
      setImageLoading(false);
    } else {
      setImageLoading(true);
    }
  }, [currentPageUrl]);

  usePageTitle(title ? `Reading: ${title}` : "Reader");

  // Fetch chapters
  const { data: chapters = [] } = useFileChapters(fileId);
  const flatChapters = useMemo(() => flattenChapters(chapters), [chapters]);

  // Fetch and update viewer settings
  const { data: settings, isLoading: settingsLoading } = useUserSettings();
  const updateSettings = useUpdateUserSettings();
  const preloadCount = settings?.preload_count ?? 3;
  const fitMode = settings?.fit_mode ?? "fit-height";
  const hideChrome = settings?.viewer_hide_chrome ?? false;
  const settingsReady = !settingsLoading && settings != null;
  // Failures share one toast id, so quick changes that all fail show one
  // toast instead of a stack.
  const saveSetting = (payload: UserSettingsPayload, onFailure?: () => void) =>
    updateSettings.mutate(payload, {
      onError: (error) => {
        onFailure?.();
        toastRequestError(error, "Failed to save reader settings", {
          id: "reader-settings-error",
        });
      },
    });
  // The preload slider shows a local draft while it is dragged and saves once
  // on release. The draft yields to the saved value when that changes, and is
  // dropped if the save fails so the slider shows what is actually stored.
  const [preloadDraft, setPreloadDraft] = useState<number | null>(null);
  useEffect(() => {
    setPreloadDraft(null);
  }, [preloadCount]);

  const { chromeVisible, toggleChrome } = useAutoHideChrome(hideChrome);

  // Sync URL with current page
  useEffect(() => {
    const urlPage = parseInt(searchParams.get("page") || "0", 10);
    if (urlPage !== currentPage) {
      setSearchParams({ page: currentPage.toString() }, { replace: true });
    }
  }, [currentPage, searchParams, setSearchParams]);

  // Navigate to page
  const goToPage = useCallback(
    (page: number) => {
      if (page < 0) return;
      if (page >= totalPages) {
        // Navigate back to book detail
        navigate(`/libraries/${libraryId}/books/${bookId}`);
        return;
      }
      setCurrentPage(page);
    },
    [totalPages, navigate, libraryId, bookId],
  );

  // Keyboard navigation
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "ArrowRight" || e.key === "d" || e.key === "D") {
        goToPage(currentPage + 1);
      } else if (e.key === "ArrowLeft" || e.key === "a" || e.key === "A") {
        goToPage(currentPage - 1);
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [currentPage, goToPage]);

  // Preload pages
  const preloadedPages = useMemo(() => {
    const pages: number[] = [];
    for (
      let i = Math.max(0, currentPage - preloadCount);
      i <= Math.min(totalPages - 1, currentPage + preloadCount);
      i++
    ) {
      pages.push(i);
    }
    return pages;
  }, [currentPage, preloadCount, totalPages]);

  // Find current chapter
  const currentChapter = useMemo(() => {
    const filtered = flatChapters.filter(
      (ch) => ch.start_page != null && ch.start_page <= currentPage,
    );
    return filtered[filtered.length - 1];
  }, [flatChapters, currentPage]);

  // Progress percentage
  const progressPercent =
    totalPages > 1 ? (currentPage / (totalPages - 1)) * 100 : 0;

  // Handle progress bar click
  const handleProgressClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const percent = x / rect.width;
    const targetPage = Math.round(percent * (totalPages - 1));
    goToPage(Math.max(0, Math.min(targetPage, totalPages - 1)));
  };

  const handleProgressKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Home") goToPage(0);
    else if (e.key === "End") goToPage(totalPages - 1);
    else return;
    e.preventDefault();
  };

  return (
    <div className="fixed inset-x-0 bottom-0 top-[var(--demo-banner-height,0px)] bg-background flex flex-col">
      {/* Header */}
      <header
        className={cn(
          "flex items-center justify-end px-4 py-2 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60",
          hideChrome &&
            "fixed top-[var(--demo-banner-height,0px)] inset-x-0 z-20 transition-transform duration-300",
          hideChrome && !chromeVisible && "-translate-y-full",
        )}
      >
        <div className="flex items-center gap-2">
          {/* Chapter dropdown */}
          {flatChapters.length > 0 && (
            <select
              aria-label="Jump to chapter"
              className="text-sm bg-transparent border rounded px-2 py-1"
              onChange={(e) => {
                const ch = flatChapters.find(
                  (c) => c.id === Number(e.target.value),
                );
                if (ch?.start_page != null) {
                  goToPage(ch.start_page);
                }
              }}
              value={currentChapter?.id ?? ""}
            >
              {flatChapters.map((ch) => (
                <option key={ch.id} value={ch.id}>
                  {ch.title}
                </option>
              ))}
            </select>
          )}

          {/* Settings */}
          <Popover>
            <PopoverTrigger asChild>
              <Button aria-label="Reader settings" size="icon" variant="ghost">
                <Settings className="h-4 w-4" />
              </Button>
            </PopoverTrigger>
            <PopoverContent align="end" className="w-64">
              <div className="space-y-4">
                <div>
                  <label className="text-sm font-medium">
                    Preload Count: {preloadDraft ?? preloadCount}
                  </label>
                  <Slider
                    className="mt-2"
                    disabled={!settingsReady}
                    max={10}
                    min={1}
                    onValueChange={([value]) => setPreloadDraft(value)}
                    onValueCommit={([value]) =>
                      saveSetting({ preload_count: value }, () =>
                        setPreloadDraft(null),
                      )
                    }
                    step={1}
                    thumbLabel="Preload count"
                    value={[preloadDraft ?? preloadCount]}
                  />
                </div>
                <div>
                  <label
                    className="text-sm font-medium"
                    id="page-fit-mode-label"
                  >
                    Fit Mode
                  </label>
                  <div
                    aria-labelledby="page-fit-mode-label"
                    className="flex gap-2 mt-2"
                    role="group"
                  >
                    <Button
                      aria-pressed={fitMode === "fit-height"}
                      disabled={!settingsReady}
                      onClick={() => saveSetting({ fit_mode: "fit-height" })}
                      size="sm"
                      variant={fitMode === "fit-height" ? "default" : "outline"}
                    >
                      Fit Height
                    </Button>
                    <Button
                      aria-pressed={fitMode === "fit-width"}
                      disabled={!settingsReady}
                      onClick={() => saveSetting({ fit_mode: "fit-width" })}
                      size="sm"
                      variant={fitMode === "fit-width" ? "default" : "outline"}
                    >
                      Fit Width
                    </Button>
                  </div>
                </div>
                <div className="flex items-center justify-between">
                  <label className="text-sm font-medium" htmlFor="hide-chrome">
                    Auto-hide controls
                  </label>
                  <Switch
                    checked={hideChrome}
                    disabled={!settingsReady}
                    id="hide-chrome"
                    onCheckedChange={(checked) =>
                      saveSetting({ viewer_hide_chrome: checked })
                    }
                  />
                </div>
              </div>
            </PopoverContent>
          </Popover>
        </div>
      </header>

      {/* Page Display */}
      <main
        className={cn(
          "flex bg-black relative",
          hideChrome
            ? "fixed inset-x-0 bottom-0 top-[var(--demo-banner-height,0px)]"
            : "flex-1",
          fitMode === "fit-width"
            ? "items-start justify-start overflow-auto"
            : "items-center justify-center overflow-hidden",
        )}
        ref={mainRef}
      >
        {/* Tap zones for mobile navigation. They duplicate the labeled
            footer buttons for pointer users only, so assistive tech, the
            tab order, and mouse focus skip them. */}
        <Button
          aria-hidden
          className="absolute left-0 top-0 w-1/3 h-full z-10 opacity-0"
          disabled={currentPage === 0}
          onClick={() => goToPage(currentPage - 1)}
          onMouseDown={(e) => e.preventDefault()}
          tabIndex={-1}
          variant="ghost"
        />
        <Button
          aria-hidden
          className="absolute right-0 top-0 w-1/3 h-full z-10 opacity-0"
          onClick={() => goToPage(currentPage + 1)}
          onMouseDown={(e) => e.preventDefault()}
          tabIndex={-1}
          variant="ghost"
        />

        {/* Center tap zone: toggle chrome on mobile */}
        {hideChrome && (
          <Button
            aria-label="Toggle controls"
            className="absolute left-1/3 top-0 w-1/3 h-full z-10 opacity-0"
            onClick={toggleChrome}
            variant="ghost"
          />
        )}

        {/* Loading spinner */}
        {imageLoading && (
          <div className="absolute inset-0 flex items-center justify-center z-[5] bg-black/60">
            <LoadingSpinner />
          </div>
        )}

        <img
          alt={`Page ${currentPage + 1}`}
          className={
            fitMode === "fit-height"
              ? "max-h-full w-auto object-contain"
              : "w-full h-auto"
          }
          onError={() => setImageLoading(false)}
          onLoad={() => setImageLoading(false)}
          ref={imgRef}
          src={currentPageUrl}
        />
        {/* Preloaded images (hidden) */}
        {preloadedPages
          .filter((p) => p !== currentPage)
          .map((p) => (
            <link as="image" href={getPageUrl(p)} key={p} rel="prefetch" />
          ))}
      </main>

      {/* Controls */}
      <footer
        className={cn(
          "border-t bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60",
          hideChrome &&
            "fixed bottom-0 inset-x-0 z-20 transition-transform duration-300",
          hideChrome && !chromeVisible && "translate-y-full",
        )}
      >
        {/* Progress Bar. The window's arrow-key handler already turns pages,
            so the bar adds only Home and End for keyboard seeking. */}
        <div className="px-4 pt-3">
          <div
            aria-label="Reading progress"
            aria-valuemax={totalPages}
            aria-valuemin={1}
            aria-valuenow={currentPage + 1}
            aria-valuetext={`Page ${currentPage + 1} of ${totalPages}`}
            className="relative h-1.5 bg-muted rounded-full cursor-pointer focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            onClick={handleProgressClick}
            onKeyDown={handleProgressKeyDown}
            role="slider"
            tabIndex={0}
          >
            <div
              className="absolute inset-y-0 left-0 bg-primary rounded-full"
              style={{ width: `${progressPercent}%` }}
            />
            {/* Chapter markers */}
            {flatChapters.map((ch) => {
              if (ch.start_page == null || totalPages <= 1) return null;
              const pos = (ch.start_page / (totalPages - 1)) * 100;
              return (
                <div
                  className="absolute top-1/2 -translate-y-1/2 w-0.5 h-2.5 bg-muted-foreground/50"
                  key={ch.id}
                  style={{ left: `${pos}%` }}
                  title={ch.title}
                />
              );
            })}
          </div>
          {currentChapter && (
            <div className="text-xs text-muted-foreground mt-1">
              {currentChapter.title}
            </div>
          )}
        </div>

        {/* Navigation buttons */}
        <div className="flex items-center justify-between px-4 py-2">
          <Button
            aria-label="Previous page"
            disabled={currentPage === 0}
            onClick={() => goToPage(currentPage - 1)}
            size="icon"
            variant="ghost"
          >
            <ChevronLeft className="h-5 w-5" />
          </Button>
          <span className="text-sm text-muted-foreground">
            Page {currentPage + 1} of {totalPages}
          </span>
          <Button
            aria-label="Next page"
            onClick={() => goToPage(currentPage + 1)}
            size="icon"
            variant="ghost"
          >
            <ChevronRight className="h-5 w-5" />
          </Button>
        </div>
      </footer>
    </div>
  );
}
