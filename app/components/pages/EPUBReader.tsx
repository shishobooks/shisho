import {
  AlertCircle,
  ChevronLeft,
  ChevronRight,
  Loader2,
  Settings,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Slider } from "@/components/ui/slider";
import { useEpubBlob } from "@/hooks/queries/epub";
import {
  useUpdateUserSettings,
  useUserSettings,
  type UpdateUserSettingsVariables,
} from "@/hooks/queries/settings";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { File } from "@/types";

import "@/libraries/foliate/view.js";

interface EPUBReaderProps {
  file: File;
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

export default function EPUBReader({ file, bookTitle }: EPUBReaderProps) {
  usePageTitle(bookTitle ? `Reading: ${bookTitle}` : "Reader");

  const {
    data: blob,
    isLoading,
    isError,
    error,
    refetch,
  } = useEpubBlob(file.id);

  const { data: settings, isLoading: settingsLoading } = useUserSettings();
  const updateSettings = useUpdateUserSettings();
  const settingsReady = !settingsLoading && settings != null;

  const fontSize = settings?.viewer_epub_font_size ?? 100;
  const theme = settings?.viewer_epub_theme ?? "light";
  const flow = settings?.viewer_epub_flow ?? "paginated";

  // Local draft state lets the slider thumb and label update live while the
  // user drags, without firing a PUT on every tick. We only commit to the API
  // on `onValueCommit` (pointer up). The draft is reset to the server value
  // whenever the canonical `fontSize` changes so outside updates flow in.
  const [fontSizeDraft, setFontSizeDraft] = useState(fontSize);
  useEffect(() => {
    setFontSizeDraft(fontSize);
  }, [fontSize]);

  const commitSettings = useCallback(
    (partial: UpdateUserSettingsVariables) => {
      updateSettings.mutate(partial);
    },
    [updateSettings],
  );

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
  const [loadError, setLoadError] = useState<Error | null>(null);

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
    if (!view) return;
    if (typeof view.open !== "function") {
      setLoadError(
        new Error(
          "EPUB renderer did not register — the <foliate-view> custom element is missing its open() method.",
        ),
      );
      return;
    }
    const viewInit = (
      view as HTMLElement & {
        init?: (opts: {
          lastLocation?: unknown;
          showTextStart?: boolean;
        }) => Promise<void>;
      }
    ).init;

    let cancelled = false;
    setBookReady(false);
    setLoadError(null);

    // Hardcoded name. foliate doesn't surface the synthetic File's name
    // anywhere user-visible — using bookTitle would just cause the effect
    // to re-fire when useBook resolves (undefined → real value), launching
    // a second overlapping open() on the same element.
    const bookFile = new globalThis.File([blob], "book.epub", {
      type: "application/epub+zip",
    });

    (async () => {
      await view.open!(bookFile);
      if (cancelled) return;
      setToc(flattenToc(view.book?.toc));
      // foliate's open() only parses; init() is what actually navigates to
      // the first page and triggers rendering. Without it the view stays at
      // 0% with an empty content area.
      if (viewInit) {
        await viewInit.call(view, { showTextStart: false });
        if (cancelled) return;
      }
      setBookReady(true);
    })().catch((err: unknown) => {
      if (cancelled) return;
      const asError = err instanceof Error ? err : new Error(String(err));
      console.error("EPUBReader: foliate open() failed", err);
      setLoadError(asError);
    });

    return () => {
      cancelled = true;
      // Use the captured `view` reference from this effect's run, not
      // viewRef.current — they're the same element in practice, but reading
      // the ref again in cleanup would couple us to whatever element the
      // ref happens to point at later.
      // close() tears down foliate's paginator, which throws if called
      // before a book finished opening. Swallow — cleanup failures
      // shouldn't take down the reader.
      try {
        (view as HTMLElement & { close?: () => void }).close?.();
      } catch {
        // no-op
      }
    };
  }, [blob]);

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

  // Push viewer settings (font size / theme / flow) to the foliate renderer.
  useEffect(() => {
    if (!bookReady) return;
    const view = viewRef.current as
      | (HTMLElement & {
          renderer?: {
            setStyles?: (styles: string | [string, string]) => void;
            setAttribute?: (name: string, value: string) => void;
          };
        })
      | null;
    const renderer = view?.renderer;
    if (!renderer) return;

    const { fg, bg } =
      theme === "dark"
        ? { fg: "#e8e8e8", bg: "#1a1a1a" }
        : theme === "sepia"
          ? { fg: "#5b4636", bg: "#f4ecd8" }
          : { fg: "#111111", bg: "#ffffff" };

    // foliate's `setStyles` takes a CSS string (or [beforeStyle, style] tuple).
    // See app/libraries/foliate/paginator.js `setStyles(styles)`. Books ship
    // their own stylesheets with more specific selectors, so mark the
    // user-visible theme properties as `!important` to ensure the selected
    // theme overrides book-provided styles. This mirrors foliate's own theming
    // (`setStylesImportant` in paginator.js).
    const css = `
      @namespace epub "http://www.idpf.org/2007/ops";
      html {
        color: ${fg} !important;
        background: ${bg} !important;
      }
      html, body {
        font-size: ${fontSize}% !important;
      }
    `;
    renderer.setStyles?.(css);
    renderer.setAttribute?.("flow", flow);
  }, [bookReady, fontSize, theme, flow]);

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

  if (isError || loadError) {
    const displayError = loadError ?? error;
    return (
      <div className="fixed inset-0 bg-background flex flex-col items-center justify-center gap-4 p-4 text-center">
        <AlertCircle className="h-8 w-8 text-destructive" />
        <div>
          <p className="font-medium">We couldn't load this book.</p>
          <p className="text-sm text-muted-foreground mt-1">
            {displayError?.message ?? "Unknown error"}
          </p>
        </div>
        <Button
          onClick={() => {
            setLoadError(null);
            refetch();
          }}
          variant="default"
        >
          Retry
        </Button>
      </div>
    );
  }

  return (
    <div className="fixed inset-0 bg-background flex flex-col">
      <header className="flex items-center justify-end px-4 py-2 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="flex items-center gap-2">
          {toc.length > 0 && (
            <select
              className="text-sm bg-transparent border rounded px-2 py-1 cursor-pointer"
              onChange={(e) => handleTocChange(e.target.value)}
              value={currentTocHref ?? ""}
            >
              {currentTocHref === null && <option value="">—</option>}
              {toc.map((entry, index) => (
                <option key={`${index}-${entry.href}`} value={entry.href}>
                  {entry.label}
                </option>
              ))}
            </select>
          )}
          <Popover>
            <PopoverTrigger asChild>
              <Button aria-label="Settings" size="icon" variant="ghost">
                <Settings className="h-4 w-4" />
              </Button>
            </PopoverTrigger>
            <PopoverContent align="end" className="w-64">
              <div className="space-y-4">
                <div>
                  <label className="text-sm font-medium">
                    Font size: {fontSizeDraft}%
                  </label>
                  <Slider
                    className="mt-2"
                    disabled={!settingsReady}
                    max={200}
                    min={50}
                    onValueChange={([value]) => setFontSizeDraft(value)}
                    onValueCommit={([value]) =>
                      commitSettings({ viewer_epub_font_size: value })
                    }
                    step={10}
                    value={[fontSizeDraft]}
                  />
                </div>
                <div>
                  <label className="text-sm font-medium">Theme</label>
                  <div className="flex gap-2 mt-2">
                    {(["light", "dark", "sepia"] as const).map((t) => (
                      <Button
                        disabled={!settingsReady}
                        key={t}
                        onClick={() => commitSettings({ viewer_epub_theme: t })}
                        size="sm"
                        variant={theme === t ? "default" : "outline"}
                      >
                        {t.charAt(0).toUpperCase() + t.slice(1)}
                      </Button>
                    ))}
                  </div>
                </div>
                <div>
                  <label className="text-sm font-medium">Flow</label>
                  <div className="flex gap-2 mt-2">
                    {(["paginated", "scrolled"] as const).map((f) => (
                      <Button
                        disabled={!settingsReady}
                        key={f}
                        onClick={() => commitSettings({ viewer_epub_flow: f })}
                        size="sm"
                        variant={flow === f ? "default" : "outline"}
                      >
                        {f.charAt(0).toUpperCase() + f.slice(1)}
                      </Button>
                    ))}
                  </div>
                </div>
              </div>
            </PopoverContent>
          </Popover>
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

        <Button
          aria-label="Previous page"
          className="absolute left-0 top-0 w-1/3 h-full z-10 opacity-0"
          onClick={goPrev}
          variant="ghost"
        />
        <Button
          aria-label="Next page"
          className="absolute right-0 top-0 w-1/3 h-full z-10 opacity-0"
          onClick={goNext}
          variant="ghost"
        />

        <foliate-view
          ref={(el) => {
            viewRef.current = el;
          }}
          style={{
            display: "block",
            position: "absolute",
            inset: 0,
          }}
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
