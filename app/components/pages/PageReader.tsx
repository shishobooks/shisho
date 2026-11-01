import { ChevronLeft, ChevronRight, Settings } from "lucide-react";
import React, {
  memo,
  useCallback,
  useEffect,
  useEffectEvent,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useNavigate, useSearchParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import ReaderChapterSelect from "@/components/library/ReaderChapterSelect";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { useFileChapters } from "@/hooks/queries/chapters";
import { useBookReading, useSetPageFlow } from "@/hooks/queries/reading";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { useAutoHideChrome } from "@/hooks/useAutoHideChrome";
import { pagesToProbe, useDetectedPageFlow } from "@/hooks/useDetectedPageFlow";
import { usePageTitle } from "@/hooks/usePageTitle";
import { toastRequestError } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import {
  ThumbnailSize256,
  type Chapter,
  type PageFlow,
  type ThumbnailSize,
  type UserSettingsPayload,
} from "@/types";
import { pageTurnForKey } from "@/utils/readerKeys";

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

// Big enough that rounding moves a page's measured shape by under 1%.
const PROBE_THUMBNAIL_SIZE = ThumbnailSize256;

// How long the scrolled flow waits for the page to settle before writing it
// to the URL. Browsers cap how often a page may rewrite its URL, and
// scrolling can change the page every frame.
const URL_SETTLE_MS = 300;

const FLOW_LABELS: Record<PageFlow, string> = {
  paginated: "Paginated",
  scrolled: "Scrolled",
};

// Failures share one toast id, so quick changes that all fail show one toast
// instead of a stack.
const toastSaveError = (error: unknown) =>
  toastRequestError(error, "Failed to save reader settings", {
    id: "reader-settings-error",
  });

// In the scrolled flow, the page crossing this line (a fraction of the view's
// height from its top) is the current page.
const CURRENT_PAGE_LINE = 0.1;

// The scrolled flow's current page: the one crossing the line, or the last
// page once scrolled to the end, since a short last page may never reach it.
const pageInView = (
  scroller: HTMLElement,
  pages: (HTMLElement | null)[],
): number => {
  if (scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - 1) {
    return pages.length - 1;
  }
  const line =
    scroller.getBoundingClientRect().top +
    scroller.clientHeight * CURRENT_PAGE_LINE;
  let low = 0;
  let high = pages.length - 1;
  while (low < high) {
    const mid = Math.ceil((low + high) / 2);
    const top = pages[mid]?.getBoundingClientRect().top ?? Infinity;
    if (top <= line) low = mid;
    else high = mid - 1;
  }
  return low;
};

interface ScrolledPageProps {
  alt: string;
  src: string;
  scrollerRef: React.RefObject<HTMLElement | null>;
  ref: (el: HTMLDivElement | null) => void;
}

// One page of the scrolled flow. Until its image loads it holds a screen's
// height, so lazy loading leaves the pages far below alone. The reader turns
// off the browser's scroll anchoring, which Safari lacks, so a page wholly
// above the view that grows on load moves the view down by as much itself,
// keeping what the reader is looking at in place.
const ScrolledPage = memo(function ScrolledPage({
  alt,
  src,
  scrollerRef,
  ref,
}: ScrolledPageProps) {
  const boxRef = useRef<HTMLDivElement | null>(null);
  const [loaded, setLoaded] = useState(false);
  const placeholder = useRef<{ height: number; above: boolean } | null>(null);

  useLayoutEffect(() => {
    if (!loaded) return;
    const box = boxRef.current;
    const scroller = scrollerRef.current;
    const before = placeholder.current;
    placeholder.current = null;
    if (!before?.above || !box || !scroller) return;
    scroller.scrollTop += box.offsetHeight - before.height;
  }, [loaded, scrollerRef]);

  // The image stays out of the layout until it loads, so the box still has
  // its placeholder height here.
  const handleDone = () => {
    const box = boxRef.current;
    const scroller = scrollerRef.current;
    if (box && scroller) {
      placeholder.current = {
        height: box.offsetHeight,
        above:
          box.getBoundingClientRect().bottom <=
          scroller.getBoundingClientRect().top + 1,
      };
    }
    setLoaded(true);
  };

  return (
    <div
      className={cn("relative", !loaded && "h-dvh")}
      ref={(el) => {
        boxRef.current = el;
        ref(el);
      }}
    >
      {!loaded && (
        <div className="absolute inset-0 flex items-center justify-center">
          <LoadingSpinner />
        </div>
      )}
      <img
        alt={alt}
        className={
          loaded
            ? "block h-auto w-full"
            : "absolute inset-0 h-full w-full opacity-0"
        }
        loading="lazy"
        onError={handleDone}
        onLoad={handleDone}
        src={src}
      />
    </div>
  );
});

interface PageReaderProps {
  fileId: number;
  bookId: number;
  libraryId: string;
  totalPages: number;
  // A page's image URL, or its thumbnail's when given a size.
  getPageUrl: (pageNum: number, size?: ThumbnailSize) => string;
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

  // Parse page from URL, default to 0, and keep it within the file
  const [currentPage, setCurrentPage] = useState(() => {
    const urlPage = parseInt(searchParams.get("page") || "0", 10);
    if (isNaN(urlPage)) return 0;
    return Math.max(0, Math.min(urlPage, totalPages - 1));
  });

  // Image loading state
  const [imageLoading, setImageLoading] = useState(true);
  const currentPageUrl = getPageUrl(currentPage);
  const imgRef = useRef<HTMLImageElement>(null);
  const mainRef = useRef<HTMLElement>(null);
  const pageRefs = useRef<(HTMLElement | null)[]>([]);

  // The flow: the one chosen while open, else the one saved for the Book,
  // else the one the page shapes suggest. Undefined until it is known. The
  // flow is a per-Book choice: some comics are made for vertical scrolling.
  const readingQuery = useBookReading(bookId);
  const setPageFlow = useSetPageFlow();
  const [chosenFlow, setChosenFlow] = useState<PageFlow | null>(null);
  // A failed load leaves nothing saved, and the page shapes decide.
  const savedFlowKnown = !readingQuery.isLoading;
  const savedFlow = readingQuery.data?.page_flow ?? null;
  // Thumbnails keep the page's shape at a fraction of its bytes, so a slow
  // connection still learns the shape before the probe gives up.
  const detectedFlow = useDetectedPageFlow(
    savedFlowKnown && !savedFlow && !chosenFlow
      ? pagesToProbe(totalPages).map((page) =>
          getPageUrl(page, PROBE_THUMBNAIL_SIZE),
        )
      : null,
  );
  const flow: PageFlow | undefined =
    chosenFlow ?? (savedFlowKnown ? (savedFlow ?? detectedFlow) : undefined);
  const scrolled = flow === "scrolled";

  // Reset scroll position on page change (layout effect to avoid flash).
  // The scrolled flow moves the view itself.
  useLayoutEffect(() => {
    if (!scrolled) mainRef.current?.scrollTo(0, 0);
  }, [currentPage, scrolled]);

  // Entering the scrolled flow shows the current page, and takes focus so
  // the arrow keys, Space, and Page Down scroll the pages.
  const showCurrentPage = useEffectEvent(() => {
    pageRefs.current[currentPage]?.scrollIntoView({ block: "start" });
    mainRef.current?.focus({ preventScroll: true });
  });
  useLayoutEffect(() => {
    if (scrolled) showCurrentPage();
  }, [scrolled]);

  // Scrolling moves the current page, at most once a frame.
  const scrollFrame = useRef(0);
  useEffect(() => () => cancelAnimationFrame(scrollFrame.current), []);
  const handleScroll = () => {
    if (scrollFrame.current) return;
    scrollFrame.current = requestAnimationFrame(() => {
      scrollFrame.current = 0;
      const main = mainRef.current;
      if (!main) return;
      setCurrentPage(pageInView(main, pageRefs.current.slice(0, totalPages)));
    });
  };

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
  const saveSetting = (payload: UserSettingsPayload, onFailure?: () => void) =>
    updateSettings.mutate(payload, {
      onError: (error) => {
        onFailure?.();
        toastSaveError(error);
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

  const chooseFlow = (next: PageFlow) => {
    setChosenFlow(next);
    setPageFlow.mutate(
      { bookId, payload: { page_flow: next } },
      {
        onError: (error) => {
          setChosenFlow(null);
          toastSaveError(error);
        },
      },
    );
  };

  // Sync URL with current page, once it settles in the scrolled flow
  useEffect(() => {
    const urlPage = parseInt(searchParams.get("page") || "0", 10);
    if (urlPage === currentPage) return;
    const write = () =>
      setSearchParams({ page: currentPage.toString() }, { replace: true });
    if (!scrolled) {
      write();
      return;
    }
    const timer = setTimeout(write, URL_SETTLE_MS);
    return () => clearTimeout(timer);
  }, [currentPage, scrolled, searchParams, setSearchParams]);

  // One stable ref callback per page, so a page change doesn't re-render
  // every page of the scrolled flow.
  const pageRefSetters = useMemo(
    () =>
      Array.from(
        { length: totalPages },
        (_, page) => (el: HTMLElement | null) => {
          pageRefs.current[page] = el;
        },
      ),
    [totalPages],
  );

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
      if (scrolled) {
        pageRefs.current[page]?.scrollIntoView({ block: "start" });
      }
    },
    [totalPages, navigate, libraryId, bookId, scrolled],
  );

  // Keyboard navigation
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const turn = pageTurnForKey(e);
      if (turn === "next") goToPage(currentPage + 1);
      else if (turn === "prev") goToPage(currentPage - 1);
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
            <ReaderChapterSelect
              entries={flatChapters.map((ch) => ({
                value: String(ch.id),
                label: ch.title,
              }))}
              onValueChange={(value) => {
                const ch = flatChapters.find((c) => c.id === Number(value));
                if (ch?.start_page != null) {
                  goToPage(ch.start_page);
                }
              }}
              value={currentChapter ? String(currentChapter.id) : ""}
            />
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
                  <p className="text-sm font-medium">
                    Preload Count: {preloadDraft ?? preloadCount}
                  </p>
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
                  <p className="text-sm font-medium" id="page-flow-label">
                    Flow
                  </p>
                  <div
                    aria-labelledby="page-flow-label"
                    className="flex gap-2 mt-2"
                    role="group"
                  >
                    {(["paginated", "scrolled"] as const).map((f) => (
                      <Button
                        aria-pressed={flow === f}
                        disabled={!flow}
                        key={f}
                        onClick={() => chooseFlow(f)}
                        size="sm"
                        variant={flow === f ? "default" : "outline"}
                      >
                        {FLOW_LABELS[f]}
                      </Button>
                    ))}
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {chosenFlow || savedFlow
                      ? "Saved for this book."
                      : "Picked from the page shapes until you choose."}
                  </p>
                </div>
                {!scrolled && (
                  <div>
                    <p className="text-sm font-medium" id="page-fit-mode-label">
                      Fit Mode
                    </p>
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
                        variant={
                          fitMode === "fit-height" ? "default" : "outline"
                        }
                      >
                        Fit Height
                      </Button>
                      <Button
                        aria-pressed={fitMode === "fit-width"}
                        disabled={!settingsReady}
                        onClick={() => saveSetting({ fit_mode: "fit-width" })}
                        size="sm"
                        variant={
                          fitMode === "fit-width" ? "default" : "outline"
                        }
                      >
                        Fit Width
                      </Button>
                    </div>
                  </div>
                )}
                <div className="flex items-center justify-between">
                  <Label htmlFor="hide-chrome">Auto-hide controls</Label>
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
          scrolled
            ? "flex-col items-center overflow-y-auto outline-none [overflow-anchor:none]"
            : fitMode === "fit-width"
              ? "items-start justify-start overflow-auto"
              : "items-center justify-center overflow-hidden",
        )}
        onScroll={scrolled ? handleScroll : undefined}
        ref={mainRef}
        tabIndex={scrolled ? -1 : undefined}
      >
        {!flow && (
          <div className="m-auto">
            <LoadingSpinner />
          </div>
        )}

        {/* Scrolled flow: every page in one column. With auto-hide on, a
            tap anywhere toggles the controls, since there are no tap zones
            to turn pages; keyboard users get a button that shows on focus. */}
        {scrolled && hideChrome && (
          <Button
            className="sr-only focus-visible:not-sr-only focus-visible:fixed focus-visible:bottom-24 focus-visible:left-1/2 focus-visible:z-30 focus-visible:-translate-x-1/2"
            onClick={toggleChrome}
            variant="secondary"
          >
            Toggle controls
          </Button>
        )}
        {scrolled && (
          <div
            className="w-full max-w-3xl"
            onClick={hideChrome ? toggleChrome : undefined}
          >
            {Array.from({ length: totalPages }, (_, page) => (
              <ScrolledPage
                alt={`Page ${(page + 1).toLocaleString()}`}
                key={page}
                ref={pageRefSetters[page]}
                scrollerRef={mainRef}
                src={getPageUrl(page)}
              />
            ))}
          </div>
        )}

        {flow === "paginated" && (
          <>
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
              alt={`Page ${(currentPage + 1).toLocaleString()}`}
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
          </>
        )}

        {/* Preloaded images (hidden). The scrolled flow's lazy images
            already load the pages near the view. */}
        {!scrolled &&
          preloadedPages
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
            aria-valuetext={`Page ${(currentPage + 1).toLocaleString()} of ${totalPages.toLocaleString()}`}
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
            Page {(currentPage + 1).toLocaleString()} of{" "}
            {totalPages.toLocaleString()}
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
