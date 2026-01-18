import { ArrowLeft, ChevronLeft, ChevronRight, Settings } from "lucide-react";
import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Link,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Slider } from "@/components/ui/slider";
import { useBook } from "@/hooks/queries/books";
import { useFileChapters } from "@/hooks/queries/chapters";
import {
  useUpdateViewerSettings,
  useViewerSettings,
} from "@/hooks/queries/settings";
import type { Chapter } from "@/types";

// Flatten chapters for progress bar (CBZ chapters don't nest)
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

export default function CBZReader() {
  const { libraryId, bookId, fileId } = useParams<{
    libraryId: string;
    bookId: string;
    fileId: string;
  }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  // Parse page from URL, default to 0
  const urlPage = parseInt(searchParams.get("page") || "0", 10);
  const [currentPage, setCurrentPage] = useState(isNaN(urlPage) ? 0 : urlPage);

  // Fetch book data for page count
  const { data: book, isLoading: bookLoading } = useBook(bookId);
  const file = book?.files?.find((f) => f.id === Number(fileId));
  const pageCount = file?.page_count || 0;

  // Fetch chapters
  const { data: chapters = [] } = useFileChapters(
    fileId ? Number(fileId) : undefined,
  );
  const flatChapters = useMemo(() => flattenChapters(chapters), [chapters]);

  // Fetch and update viewer settings
  const { data: settings, isLoading: settingsLoading } = useViewerSettings();
  const updateSettings = useUpdateViewerSettings();
  const preloadCount = settings?.preload_count ?? 3;
  const fitMode = settings?.fit_mode ?? "fit-height";
  const settingsReady = !settingsLoading && settings != null;

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
      if (page >= pageCount) {
        // Navigate back to book detail
        navigate(`/libraries/${libraryId}/books/${bookId}`);
        return;
      }
      setCurrentPage(page);
    },
    [pageCount, navigate, libraryId, bookId],
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
      i <= Math.min(pageCount - 1, currentPage + preloadCount);
      i++
    ) {
      pages.push(i);
    }
    return pages;
  }, [currentPage, preloadCount, pageCount]);

  // Find current chapter
  const currentChapter = useMemo(() => {
    const filtered = flatChapters.filter(
      (ch) => ch.start_page != null && ch.start_page <= currentPage,
    );
    return filtered[filtered.length - 1];
  }, [flatChapters, currentPage]);

  // Progress percentage
  const progressPercent =
    pageCount > 1 ? (currentPage / (pageCount - 1)) * 100 : 0;

  // Handle progress bar click
  const handleProgressClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const percent = x / rect.width;
    const targetPage = Math.round(percent * (pageCount - 1));
    goToPage(Math.max(0, Math.min(targetPage, pageCount - 1)));
  };

  // Build page URL
  const pageUrl = (page: number) => `/api/books/files/${fileId}/page/${page}`;

  // Show loading state while book data is being fetched
  if (bookLoading) {
    return (
      <div className="fixed inset-0 bg-background flex items-center justify-center">
        <LoadingSpinner />
      </div>
    );
  }

  return (
    <div className="fixed inset-0 bg-background flex flex-col">
      {/* Header */}
      <header className="flex items-center justify-between px-4 py-2 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <Link
          className="flex items-center gap-2 text-muted-foreground hover:text-foreground"
          to={`/libraries/${libraryId}/books/${bookId}`}
        >
          <ArrowLeft className="h-4 w-4" />
          <span className="text-sm">Back</span>
        </Link>

        <div className="flex items-center gap-2">
          {/* Chapter dropdown */}
          {flatChapters.length > 0 && (
            <select
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
              <Button size="icon" variant="ghost">
                <Settings className="h-4 w-4" />
              </Button>
            </PopoverTrigger>
            <PopoverContent align="end" className="w-64">
              <div className="space-y-4">
                <div>
                  <label className="text-sm font-medium">
                    Preload Count: {preloadCount}
                  </label>
                  <Slider
                    className="mt-2"
                    disabled={!settingsReady}
                    max={10}
                    min={1}
                    onValueChange={([value]) => {
                      updateSettings.mutate({
                        preload_count: value,
                        fit_mode: fitMode,
                      });
                    }}
                    step={1}
                    value={[preloadCount]}
                  />
                </div>
                <div>
                  <label className="text-sm font-medium">Fit Mode</label>
                  <div className="flex gap-2 mt-2">
                    <Button
                      disabled={!settingsReady}
                      onClick={() =>
                        updateSettings.mutate({
                          preload_count: preloadCount,
                          fit_mode: "fit-height",
                        })
                      }
                      size="sm"
                      variant={fitMode === "fit-height" ? "default" : "outline"}
                    >
                      Fit Height
                    </Button>
                    <Button
                      disabled={!settingsReady}
                      onClick={() =>
                        updateSettings.mutate({
                          preload_count: preloadCount,
                          fit_mode: "original",
                        })
                      }
                      size="sm"
                      variant={fitMode === "original" ? "default" : "outline"}
                    >
                      Original
                    </Button>
                  </div>
                </div>
              </div>
            </PopoverContent>
          </Popover>
        </div>
      </header>

      {/* Page Display */}
      <main
        className={`flex-1 flex items-center justify-center bg-black ${
          fitMode === "original" ? "overflow-auto" : "overflow-hidden"
        }`}
      >
        <img
          alt={`Page ${currentPage + 1}`}
          className={
            fitMode === "fit-height" ? "max-h-full w-auto object-contain" : "" // original: no constraints, natural size
          }
          src={pageUrl(currentPage)}
        />
        {/* Preloaded images (hidden) */}
        {preloadedPages
          .filter((p) => p !== currentPage)
          .map((p) => (
            <link as="image" href={pageUrl(p)} key={p} rel="prefetch" />
          ))}
      </main>

      {/* Controls */}
      <footer className="border-t bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        {/* Progress Bar */}
        <div className="px-4 pt-3">
          <div
            className="relative h-1.5 bg-muted rounded-full cursor-pointer"
            onClick={handleProgressClick}
          >
            <div
              className="absolute inset-y-0 left-0 bg-primary rounded-full"
              style={{ width: `${progressPercent}%` }}
            />
            {/* Chapter markers */}
            {flatChapters.map((ch) => {
              if (ch.start_page == null || pageCount <= 1) return null;
              const pos = (ch.start_page / (pageCount - 1)) * 100;
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
            disabled={currentPage === 0}
            onClick={() => goToPage(currentPage - 1)}
            size="icon"
            variant="ghost"
          >
            <ChevronLeft className="h-5 w-5" />
          </Button>
          <span className="text-sm text-muted-foreground">
            Page {currentPage + 1} of {pageCount}
          </span>
          <Button
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
