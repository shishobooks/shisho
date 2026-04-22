import {
  AlertCircle,
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  Loader2,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { useEpubBlob } from "@/hooks/queries/epub";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { File } from "@/types";

import "@/libraries/foliate/view.js";

interface EPUBReaderProps {
  file: File;
  libraryId: string;
  bookTitle?: string;
}

interface TocEntry {
  label: string;
  href: string;
}

interface RelocateDetail {
  fraction: number;
  tocItem?: { label?: string; href?: string } | null;
  cfi?: string;
}

const flattenToc = (
  nodes:
    | Array<{ label: string; href: string; subitems?: unknown[] }>
    | undefined,
): TocEntry[] => {
  if (!nodes) return [];
  const out: TocEntry[] = [];
  for (const n of nodes) {
    if (n.href) out.push({ label: n.label, href: n.href });
    if (Array.isArray(n.subitems)) {
      out.push(...flattenToc(n.subitems as typeof nodes));
    }
  }
  return out;
};

export default function EPUBReader({
  file,
  libraryId,
  bookTitle,
}: EPUBReaderProps) {
  usePageTitle(bookTitle ? `Reading: ${bookTitle}` : "Reader");

  const {
    data: blob,
    isLoading,
    isError,
    error,
    refetch,
  } = useEpubBlob(file.id);

  const [showExtendedHint, setShowExtendedHint] = useState(false);
  useEffect(() => {
    if (!isLoading) {
      setShowExtendedHint(false);
      return;
    }
    const timer = setTimeout(() => setShowExtendedHint(true), 10_000);
    return () => clearTimeout(timer);
  }, [isLoading]);

  const viewRef = useRef<HTMLElement | null>(null);
  const [toc, setToc] = useState<TocEntry[]>([]);
  const [fraction, setFraction] = useState(0);
  const [currentTocHref, setCurrentTocHref] = useState<string | null>(null);
  const [currentTocLabel, setCurrentTocLabel] = useState<string | null>(null);
  const [bookReady, setBookReady] = useState(false);

  // Load the blob into foliate once both are available.
  useEffect(() => {
    if (!blob) return;
    const view = viewRef.current as
      | (HTMLElement & {
          open?: (book: Blob | File) => Promise<void>;
          book?: {
            toc?: Array<{ label: string; href: string; subitems?: unknown[] }>;
          };
        })
      | null;
    if (!view || typeof view.open !== "function") return;

    let cancelled = false;
    setBookReady(false);

    const bookFile = new globalThis.File(
      [blob],
      `${bookTitle ?? "book"}.epub`,
      { type: "application/epub+zip" },
    );

    (async () => {
      await view.open!(bookFile);
      if (cancelled) return;
      setToc(flattenToc(view.book?.toc));
      setBookReady(true);
    })().catch(() => {
      // Surfaced via the main error state if it fails; foliate typically rejects with a descriptive Error.
    });

    return () => {
      cancelled = true;
    };
  }, [blob, bookTitle]);

  // Wire the relocate event for progress tracking.
  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;

    const handleRelocate = (evt: Event) => {
      const detail = (evt as CustomEvent<RelocateDetail>).detail;
      if (!detail) return;
      if (typeof detail.fraction === "number") setFraction(detail.fraction);
      setCurrentTocHref(detail.tocItem?.href ?? null);
      setCurrentTocLabel(detail.tocItem?.label ?? null);
    };

    view.addEventListener("relocate", handleRelocate);
    return () => view.removeEventListener("relocate", handleRelocate);
  }, [bookReady]);

  const goPrev = useCallback(() => {
    const view = viewRef.current as
      | (HTMLElement & { goLeft?: () => void })
      | null;
    view?.goLeft?.();
  }, []);
  const goNext = useCallback(() => {
    const view = viewRef.current as
      | (HTMLElement & { goRight?: () => void })
      | null;
    view?.goRight?.();
  }, []);

  // Keyboard navigation (only when a book is loaded).
  useEffect(() => {
    if (!bookReady) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "ArrowRight" || e.key === "d" || e.key === "D") goNext();
      else if (e.key === "ArrowLeft" || e.key === "a" || e.key === "A")
        goPrev();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [bookReady, goNext, goPrev]);

  const backHref = `/libraries/${libraryId}/books/${file.book_id}`;

  const handleTocChange = (href: string) => {
    const view = viewRef.current as
      | (HTMLElement & { goTo?: (target: string) => void })
      | null;
    view?.goTo?.(href);
  };

  const handleProgressClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const target = (e.clientX - rect.left) / rect.width;
    const view = viewRef.current as
      | (HTMLElement & { goToFraction?: (f: number) => void })
      | null;
    view?.goToFraction?.(Math.max(0, Math.min(1, target)));
  };

  const progressPercent = useMemo(() => Math.round(fraction * 100), [fraction]);

  if (isError) {
    return (
      <div className="fixed inset-0 bg-background flex flex-col items-center justify-center gap-4 p-4 text-center">
        <AlertCircle className="h-8 w-8 text-destructive" />
        <div>
          <p className="font-medium">We couldn't load this book.</p>
          <p className="text-sm text-muted-foreground mt-1">
            {error?.message ?? "Unknown error"}
          </p>
        </div>
        <div className="flex gap-2">
          <Button onClick={() => refetch()} variant="default">
            Retry
          </Button>
          <Button asChild variant="outline">
            <Link to={backHref}>Back</Link>
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="fixed inset-0 bg-background flex flex-col">
      <header className="flex items-center justify-between px-4 py-2 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <Link
          className="flex items-center gap-2 text-muted-foreground hover:text-foreground"
          to={backHref}
        >
          <ArrowLeft className="h-4 w-4" />
          <span className="text-sm">Back</span>
        </Link>

        <div className="flex items-center gap-2">
          {toc.length > 0 && (
            <select
              className="text-sm bg-transparent border rounded px-2 py-1 cursor-pointer"
              onChange={(e) => handleTocChange(e.target.value)}
              value={currentTocHref ?? ""}
            >
              {currentTocHref === null && <option value="">—</option>}
              {toc.map((entry) => (
                <option key={entry.href} value={entry.href}>
                  {entry.label}
                </option>
              ))}
            </select>
          )}
          {/* Settings popover added in Task 7 */}
        </div>
      </header>

      <main className="flex-1 relative bg-background">
        {(isLoading || !blob || !bookReady) && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-background z-20">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            <p className="text-sm text-muted-foreground">Preparing book…</p>
            {showExtendedHint && (
              <p className="text-xs text-muted-foreground">
                This may take a moment for large books.
              </p>
            )}
          </div>
        )}

        <button
          aria-label="Previous page"
          className="absolute left-0 top-0 w-1/3 h-full z-10 cursor-pointer opacity-0"
          onClick={goPrev}
          type="button"
        />
        <button
          aria-label="Next page"
          className="absolute right-0 top-0 w-1/3 h-full z-10 cursor-pointer opacity-0"
          onClick={goNext}
          type="button"
        />

        <foliate-view
          ref={(el) => {
            viewRef.current = el;
          }}
          style={{ display: "block", width: "100%", height: "100%" }}
        />
      </main>

      <footer className="border-t bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="px-4 pt-3">
          <div
            className="relative h-1.5 bg-muted rounded-full cursor-pointer"
            onClick={handleProgressClick}
          >
            <div
              className="absolute inset-y-0 left-0 bg-primary rounded-full"
              style={{ width: `${progressPercent}%` }}
            />
          </div>
          {currentTocLabel && (
            <div className="text-xs text-muted-foreground mt-1">
              {currentTocLabel}
            </div>
          )}
        </div>

        <div className="flex items-center justify-between px-4 py-2">
          <Button onClick={goPrev} size="icon" variant="ghost">
            <ChevronLeft className="h-5 w-5" />
          </Button>
          <span className="text-sm text-muted-foreground">
            {progressPercent}%
          </span>
          <Button onClick={goNext} size="icon" variant="ghost">
            <ChevronRight className="h-5 w-5" />
          </Button>
        </div>
      </footer>
    </div>
  );
}
