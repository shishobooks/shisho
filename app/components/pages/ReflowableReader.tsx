import { AlertCircle, ChevronLeft, ChevronRight, Settings } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import ChapterMarkers from "@/components/library/ChapterMarkers";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import ReaderChapterSelect from "@/components/library/ReaderChapterSelect";
import ReaderSettingsPanel from "@/components/library/ReaderSettingsPanel";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Switch } from "@/components/ui/switch";
import { useReflowableBlob } from "@/hooks/queries/reflowable";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { isMouseMove, useAutoHideChrome } from "@/hooks/useAutoHideChrome";
import { useImportedFonts, useImportedFontUrl } from "@/hooks/useImportedFonts";
import { usePageTitle } from "@/hooks/usePageTitle";
import { useReaderSettings } from "@/hooks/useReaderSettings";
import { useReadingProgress } from "@/hooks/useReadingProgress";
import { useSavedPosition } from "@/hooks/useSavedPosition";
import { createSwipeHandlers, isZoomed } from "@/hooks/useSwipeTurn";
import { requestErrorMessage, toastRequestError } from "@/libraries/api";
import {
  chapterMarks as findChapterMarks,
  type ChapterMark,
} from "@/libraries/readerChapterMarkers";
import { fontFaceFor } from "@/libraries/readerFonts";
import { footnoteText, isFootnoteReference } from "@/libraries/readerFootnotes";
import {
  footnoteStyle,
  IMPORTED_FONT_PREFIX,
  readerCSS,
  readerLayout,
  recordBookAlignment,
} from "@/libraries/readerSettings";
import { ACTIVITY_EVENTS } from "@/libraries/readingSession";
import { cn } from "@/libraries/utils";
import type { File, ReadingPosition, UserSettingsPayload } from "@/types";
import { fileTitle } from "@/utils/format";
import {
  readerAtEnd,
  withinFirstScreen,
  type EndRenderer,
} from "@/utils/readerEnd";
import { pageTurnForKey } from "@/utils/readerKeys";
import { reflowableOpenTarget } from "@/utils/readingPosition";

import "@/libraries/foliate/view.js";

interface ReflowableReaderProps {
  file: File;
  bookTitle?: string;
}

interface TocEntry {
  label: string;
  // Empty for a heading, which has no location to jump to.
  href: string;
  depth: number;
}

interface RelocateDetail {
  fraction: number;
  tocItem?: { label?: string; href?: string } | null;
  cfi?: string;
  section?: { current?: number } | null;
  // Why foliate moved (a Shisho patch in view.js). "anchor" is a re-layout
  // that keeps the place, such as a resize or a font change.
  reason?: string;
}

type FoliateRenderer = EndRenderer & {
  page?: number;
  getContents?: () => Array<{ doc: Document; index?: number }>;
  setStyles?: (styles: string | [string, string]) => void;
  getAttribute?: (name: string) => string | null;
  setAttribute?: (name: string, value: string) => void;
  // Lays the shown section out again, keeping the place.
  render?: () => void;
};

// What foliate's resolveHref gives for a link: the section, and a function
// finding the place in the section's document.
interface ResolvedHref {
  index: number;
  anchor: (doc: Document) => unknown;
}

type FoliateBookView = HTMLElement & {
  renderer?: FoliateRenderer;
  goTo?: (target: string) => unknown;
  book?: {
    // Asynchronous in an AZW3 book.
    resolveHref?: (
      href: string,
    ) => ResolvedHref | null | Promise<ResolvedHref | null>;
    sections?: Array<{ createDocument?: () => Promise<Document> }>;
  };
};

// A footnote open in a popover, beside the reference that opened it.
interface OpenFootnote {
  href: string;
  paragraphs: string[];
  // The book's own font around the reference, for a note when the reader
  // keeps the book's fonts.
  bookFontFamily: string;
  // The reference's box on screen.
  rect: { left: number; top: number; width: number; height: number };
}

// Things in a book that a tap operates, which shouldn't also turn the page.
const BOOK_CONTROLS =
  "button, input, select, textarea, label, summary, audio, video";

// The screen foliate shows, which a scroll moves only once it passes a
// screen's height: reading time counts per screen.
const screenOf = (section: number | undefined, renderer?: FoliateRenderer) =>
  `${section}:${renderer?.page}`;

// Flattens foliate's table of contents into menu entries, keeping headings
// (entries without an href) so the menu shows the book's structure.
const flattenToc = (
  nodes:
    Array<{ label: string; href: string; subitems?: unknown[] }> | undefined,
  depth = 0,
): TocEntry[] => {
  if (!nodes) return [];
  const out: TocEntry[] = [];
  for (const n of nodes) {
    out.push({ label: n.label, href: n.href ?? "", depth });
    if (Array.isArray(n.subitems)) {
      out.push(...flattenToc(n.subitems as typeof nodes, depth + 1));
    }
  }
  return out;
};

export default function ReflowableReader({
  file,
  bookTitle,
}: ReflowableReaderProps) {
  const title = fileTitle(file, bookTitle);
  usePageTitle(title ? `Reading: ${title}` : "Reader");

  const {
    data: blob,
    isLoading,
    error,
    isEnabled,
    refetch,
  } = useReflowableBlob(file.id);
  const fileType = file.file_type;

  const { data: settings, isLoading: settingsLoading } = useUserSettings();
  const updateSettings = useUpdateUserSettings();
  const settingsReady = !settingsLoading && settings != null;

  // Display settings are kept in this browser; only auto-hide is saved to
  // the account, since the other readers share it.
  const [readerSettings, updateReaderSettings] = useReaderSettings(settings);
  const flow = readerSettings.layout;
  const hideChrome = settings?.viewer_hide_chrome ?? false;

  const {
    fonts: importedFonts,
    ready: importedFontsReady,
    add: importFont,
    remove: removeFont,
  } = useImportedFonts();
  const chosenImportedFont = importedFonts.find(
    (font) => IMPORTED_FONT_PREFIX + font.id === readerSettings.font,
  );
  const chosenImportedUrl = useImportedFontUrl(chosenImportedFont);
  const fontFace = useMemo(
    () =>
      fontFaceFor(
        readerSettings.font,
        chosenImportedUrl ? [chosenImportedUrl] : [],
      ),
    [readerSettings.font, chosenImportedUrl],
  );
  // The same font for a footnote popover, which this page draws itself. A
  // private name keeps it from replacing the app's own font of that name
  // (the app uses Noto Sans).
  const noteFontFace = useMemo(
    () =>
      fontFaceFor(
        readerSettings.font,
        chosenImportedUrl ? [chosenImportedUrl] : [],
        "Shisho Note Font",
      ),
    [readerSettings.font, chosenImportedUrl],
  );
  // Until an imported font's bytes are ready, the book waits rather than
  // laying out once in its own font and again in the chosen one.
  const fontPending =
    readerSettings.font.startsWith(IMPORTED_FONT_PREFIX) &&
    (!importedFontsReady || (chosenImportedFont && !chosenImportedUrl));

  const { chromeVisible, revealChrome, toggleChrome } =
    useAutoHideChrome(hideChrome);

  const {
    saved,
    settled: positionSettled,
    failed: positionFailed,
  } = useSavedPosition(file.id);
  // Read when the book opens, so a later refetch doesn't move the reader.
  const savedRef = useRef<ReadingPosition | null>(null);
  useEffect(() => {
    savedRef.current = saved;
  }, [saved]);
  const { opened, moved, recordActivity } = useReadingProgress(file.id, {
    enabled: !positionFailed,
  });
  // True while the book opens at its saved location, which isn't reading.
  const restoringRef = useRef(true);
  // The section it opened in, whose first screen isn't past the start yet.
  const openedSectionRef = useRef<number | undefined>(undefined);

  const commitSettings = useCallback(
    (partial: UserSettingsPayload) => {
      updateSettings.mutate(partial, {
        onError: (error) =>
          toastRequestError(error, "Failed to save reader settings"),
      });
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
  // Retrying after a failed load mounts a new element, so listeners on it
  // follow this rather than the ref.
  const [viewElement, setViewElement] = useState<HTMLElement | null>(null);
  const attachView = useCallback((el: HTMLElement | null) => {
    viewRef.current = el;
    setViewElement(el);
  }, []);
  // The section and page foliate last reported.
  const placeRef = useRef("");
  const headerRef = useRef<HTMLElement | null>(null);
  const footerRef = useRef<HTMLElement | null>(null);
  const [toc, setToc] = useState<TocEntry[]>([]);
  // Where each chapter starts along the progress bar.
  const [chapterMarks, setChapterMarks] = useState<ChapterMark[]>([]);
  const [fraction, setFraction] = useState(0);
  const [currentTocHref, setCurrentTocHref] = useState<string | null>(null);
  const [currentTocLabel, setCurrentTocLabel] = useState<string | null>(null);
  const [bookReady, setBookReady] = useState(false);
  // In a right-to-left book foliate's goLeft() moves forward, so the footer
  // buttons, which turn by side, swap labels.
  const [rtl, setRtl] = useState(false);
  const [loadError, setLoadError] = useState<Error | null>(null);
  // The last footnote stays after the popover closes, so it keeps its text
  // and colors while it fades out.
  const [footnote, setFootnote] = useState<OpenFootnote | null>(null);
  const [footnoteOpen, setFootnoteOpen] = useState(false);
  const closeFootnote = useCallback(() => setFootnoteOpen(false), []);
  const [settingsOpen, setSettingsOpen] = useState(false);
  // A press in the book's frame never reaches this page, where a popover
  // listens for presses outside it, so the book's listeners close them.
  const popoverOpen = settingsOpen || footnoteOpen;
  const closePopovers = useCallback(() => {
    setSettingsOpen(false);
    setFootnoteOpen(false);
  }, []);
  // An open footnote is part of the book, so it looks like the page.
  const noteStyle =
    footnote &&
    footnoteStyle(readerSettings, noteFontFace, footnote.bookFontFamily);

  // Load the blob into foliate once both it and the saved location are
  // available.
  useEffect(() => {
    if (!blob || !positionSettled) return;
    const view = viewRef.current as
      | (HTMLElement & {
          open?: (book: Blob | File) => Promise<void>;
          book?: {
            toc?: Array<{ label: string; href: string; subitems?: unknown[] }>;
            dir?: string;
          };
        })
      | null;
    if (!view) return;
    if (typeof view.open !== "function") {
      setLoadError(
        new Error(
          "Book renderer did not register: the <foliate-view> custom element is missing its open() method.",
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
    const saved = savedRef.current;
    const savedTarget = reflowableOpenTarget(saved);

    let cancelled = false;
    setBookReady(false);
    setRtl(false);
    setLoadError(null);

    // Hardcoded name. foliate doesn't surface the synthetic File's name
    // anywhere user-visible — using bookTitle would just cause the effect
    // to re-fire when useBook resolves (undefined → real value), launching
    // a second overlapping open() on the same element. foliate picks EPUB or
    // MOBI from the bytes, so the name and type are only labels.
    const bookFile = new globalThis.File([blob], `book.${fileType}`, {
      type: blob.type,
    });

    (async () => {
      await view.open!(bookFile);
      if (cancelled) return;
      setToc(flattenToc(view.book?.toc));
      setRtl(view.book?.dir === "rtl");
      // foliate's open() only parses; init() is what actually navigates to
      // the first page and triggers rendering. Without it the view stays at
      // 0% with an empty content area.
      if (viewInit) {
        restoringRef.current = true;
        let restored = savedTarget !== undefined;
        try {
          await viewInit.call(
            view,
            savedTarget
              ? { lastLocation: savedTarget }
              : { showTextStart: false },
          );
        } catch (err) {
          // A saved location this copy of the book can't resolve (the file
          // changed) opens at the start.
          if (!savedTarget) throw err;
          console.warn("ReflowableReader: saved location failed", err);
          restored = false;
          await viewInit.call(view, { showTextStart: false });
        }
        if (cancelled) return;
        restoringRef.current = false;
        // Where it opened isn't a move, so until the reader moves it reports
        // the saved place as it was: foliate's location for the page it laid
        // out differs on another screen size.
        // Only a move reaches the end, so where it opened never does.
        const location = (
          view as HTMLElement & {
            lastLocation?: {
              cfi?: string;
              fraction?: number;
              section?: { current?: number };
            } | null;
          }
        ).lastLocation;
        openedSectionRef.current = location?.section?.current;
        const screen = screenOf(
          location?.section?.current,
          (view as HTMLElement & { renderer?: FoliateRenderer }).renderer,
        );
        if (restored && saved?.cfi) {
          opened({
            cfi: saved.cfi,
            percentage: saved.percentage,
            atEnd: false,
            screen,
          });
        } else if (restored && saved && location?.cfi) {
          // Opened at the saved percentage, which has no CFI to keep: the
          // place is where foliate laid that percentage out.
          opened({
            cfi: location.cfi,
            percentage: saved.percentage,
            atEnd: false,
            screen,
          });
        } else if (location?.cfi) {
          opened({
            cfi: location.cfi,
            percentage: location.fraction ?? 0,
            atEnd: false,
            screen,
          });
        }
      }
      setBookReady(true);
    })().catch((err: unknown) => {
      if (cancelled) return;
      const asError = err instanceof Error ? err : new Error(String(err));
      console.error("ReflowableReader: foliate open() failed", err);
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
  }, [blob, fileType, positionSettled, opened]);

  const goPrev = useCallback(() => {
    const view = viewRef.current as
      (HTMLElement & { goLeft?: () => void }) | null;
    view?.goLeft?.();
  }, []);
  const goNext = useCallback(() => {
    const view = viewRef.current as
      (HTMLElement & { goRight?: () => void }) | null;
    view?.goRight?.();
  }, []);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      const turn = pageTurnForKey(e);
      if (turn === "next") goNext();
      else if (turn === "prev") goPrev();
    },
    [goNext, goPrev],
  );

  // Keyboard navigation (only when a book is loaded).
  useEffect(() => {
    if (!bookReady) return;
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [bookReady, handleKeyDown]);

  // A tap on a paginated book, at a position in the page's coordinates.
  const handlePageTap = (x: number, y: number) => {
    const view = viewRef.current;
    if (!view) return;
    const { left, width, top, bottom } = view.getBoundingClientRect();
    // Hidden controls leave the screen, so the page is what a tap on the gear
    // lands on. Where the bars were brings them back instead of turning.
    if (!chromeVisible) {
      const headerHeight = headerRef.current?.offsetHeight ?? 0;
      const footerHeight = footerRef.current?.offsetHeight ?? 0;
      if (y < top + headerHeight || y > bottom - footerHeight) {
        revealChrome();
        return;
      }
    }
    const position = (x - left) / width;
    if (position < 1 / 3) goPrev();
    else if (position > 2 / 3) goNext();
    else toggleChrome();
  };

  // Events inside the book's iframe never reach the window, so the listeners
  // that foliate's "load" event adds to each section read the latest values
  // through this ref.
  const bookEventsRef = useRef({
    flow,
    handleKeyDown,
    handlePageTap,
    revealChrome,
    toggleChrome,
    recordActivity,
    popoverOpen,
    closePopovers,
  });
  useEffect(() => {
    bookEventsRef.current = {
      flow,
      handleKeyDown,
      handlePageTap,
      revealChrome,
      toggleChrome,
      recordActivity,
      popoverOpen,
      closePopovers,
    };
  });

  // Input over the book goes to the section's document and to foliate's
  // margins. Nothing may sit on top of foliate here: it reads swipes from the
  // same two places, so an overlay would swallow them.
  useEffect(() => {
    const view = viewElement;
    if (!view) return;

    const isFixedLayout = () =>
      (view as HTMLElement & { isFixedLayout?: boolean }).isFixedLayout ===
      true;
    // A fixed-layout book can't scroll, whatever the setting says.
    const isScrolled = () =>
      bookEventsRef.current.flow === "scrolled" && !isFixedLayout();

    // foliate's paginator reads swipes itself, but its fixed-layout renderer
    // has no touch handling.
    const swipe = createSwipeHandlers((turn) =>
      turn === "next" ? goNext() : goPrev(),
    );
    const stopListening = new AbortController();
    const listenForSwipes = (target: EventTarget, signal?: AbortSignal) => {
      const on = (type: string, handler: (e: TouchEvent) => void) =>
        target.addEventListener(
          type,
          (e) => {
            if (isFixedLayout()) handler(e as TouchEvent);
          },
          { signal },
        );
      on("touchstart", swipe.onTouchStart);
      on("touchend", swipe.onTouchEnd);
      on("touchcancel", swipe.onTouchCancel);
    };
    listenForSwipes(view, stopListening.signal);
    // Keeps a sideways drag for the swipe handlers, except on a zoomed page,
    // where the browser needs it to pan.
    const updateTouchAction = () => {
      if (!isFixedLayout()) return;
      const touchAction = isZoomed() ? "" : "pan-y pinch-zoom";
      view.style.touchAction = touchAction;
      (view as HTMLElement & { renderer?: FoliateRenderer }).renderer
        ?.getContents?.()
        .forEach(({ doc }) => {
          doc.documentElement.style.touchAction = touchAction;
        });
    };
    globalThis.visualViewport?.addEventListener("resize", updateTouchAction, {
      signal: stopListening.signal,
    });

    const handleLoad = (evt: Event) => {
      const { doc } = (evt as CustomEvent<{ doc: Document }>).detail;
      recordBookAlignment(doc);
      listenForSwipes(doc);
      updateTouchAction();
      if (isFixedLayout() && !isZoomed()) {
        // foliate's list of frames may not hold this one yet.
        doc.documentElement.style.touchAction = "pan-y pinch-zoom";
      }
      let selectedAtPress = false;
      // A tap that closes a popover does only that, as a tap outside one on
      // the rest of the page does.
      let dismissedAtPress = false;
      // Input inside the book's frame never reaches the window, where the
      // reading-session tracker listens.
      for (const name of ACTIVITY_EVENTS) {
        doc.addEventListener(name, () =>
          bookEventsRef.current.recordActivity(),
        );
      }
      doc.addEventListener("pointermove", (e) => {
        if (isMouseMove(e)) bookEventsRef.current.revealChrome();
      });
      doc.addEventListener("keydown", (e) => {
        bookEventsRef.current.handleKeyDown(e);
      });
      // The press clears a selection before the click arrives, so a tap that
      // dismisses one only shows at the press.
      doc.addEventListener("pointerdown", () => {
        selectedAtPress = doc.getSelection()?.isCollapsed === false;
        dismissedAtPress = bookEventsRef.current.popoverOpen;
        bookEventsRef.current.closePopovers();
      });
      // foliate scrolls a paginated book to whatever takes focus, on the next
      // frame. A tap's press and click arrive together, so a focused link would
      // scroll back over its own jump. Tabbing to a link still focuses it.
      doc.addEventListener("mousedown", (e) => {
        const target = e.target as Element | null;
        if (!isScrolled() && target?.closest?.("a[href]")) e.preventDefault();
      });
      doc.addEventListener("click", (e) => {
        const events = bookEventsRef.current;
        // The target comes from the iframe's realm, so `instanceof Element`
        // is false for it; check for closest() instead.
        const target = e.target as Element | null;
        if (target?.closest?.("a[href]")) return;
        if (dismissedAtPress) return;
        if (doc.getSelection()?.isCollapsed === false) return;
        if (isScrolled()) {
          events.toggleChrome();
          return;
        }
        if (selectedAtPress || target?.closest?.(BOOK_CONTROLS)) return;
        // The click's position is within the frame, which is as wide as the
        // whole section and scrolled sideways (and scaled, in a fixed-layout
        // book), so it says nothing about where on screen the tap was.
        const frame = doc.defaultView?.frameElement as HTMLElement | null;
        if (!frame) return;
        const rect = frame.getBoundingClientRect();
        const scale = rect.width / frame.offsetWidth;
        events.handlePageTap(
          rect.left + e.clientX * scale,
          rect.top + e.clientY * scale,
        );
      });
    };
    // foliate's margins are on this page, so a popover closes itself on a
    // press there; the tap that closed it shouldn't also turn the page.
    let marginDismissedAtPress = false;
    const handleMarginPress = () => {
      marginDismissedAtPress = bookEventsRef.current.popoverOpen;
    };
    const handleMarginClick = (e: MouseEvent) => {
      const events = bookEventsRef.current;
      if (marginDismissedAtPress) return;
      if (isScrolled()) return;
      // A press out here leaves the frame's selection in place, so the tap
      // clears it, as a tap on the text does, instead of turning the page.
      const selections = (
        view as HTMLElement & { renderer?: FoliateRenderer }
      ).renderer
        ?.getContents?.()
        .map(({ doc }) => doc.getSelection())
        .filter((selection) => selection?.isCollapsed === false);
      if (selections?.length) {
        selections.forEach((selection) => selection?.removeAllRanges());
        return;
      }
      events.handlePageTap(e.clientX, e.clientY);
    };

    view.addEventListener("load", handleLoad);
    view.addEventListener("pointerdown", handleMarginPress);
    view.addEventListener("click", handleMarginClick);
    return () => {
      view.removeEventListener("load", handleLoad);
      view.removeEventListener("pointerdown", handleMarginPress);
      view.removeEventListener("click", handleMarginClick);
      stopListening.abort();
    };
  }, [viewElement, goNext, goPrev]);

  // Chapter markers wait for the book to open, which is when foliate knows
  // its sections' sizes. Chapters partway through a section need its
  // document read, so they can arrive a moment after the book shows.
  useEffect(() => {
    setChapterMarks([]);
    if (!bookReady || toc.length === 0) return;
    const view = viewRef.current as
      (FoliateBookView & { getSectionFractions?: () => number[] }) | null;
    if (!view?.book) return;
    let stale = false;
    findChapterMarks(
      toc,
      view.book,
      view.getSectionFractions?.() ?? [],
      () => stale,
    )
      .then((marks) => {
        if (!stale && marks) setChapterMarks(marks);
      })
      .catch((err: unknown) => {
        // The bar works without markers.
        console.warn("ReflowableReader: couldn't place chapter markers", err);
      });
    return () => {
      stale = true;
    };
  }, [bookReady, toc]);

  // A link to a footnote opens the note in a popover instead of jumping
  // there. foliate reports every link it's about to follow, and a canceled
  // event stays put.
  useEffect(() => {
    const view = viewElement as FoliateBookView | null;
    if (!view) return;
    let latest = 0;

    const showFootnote = async (a: Element, href: string) => {
      const request = ++latest;
      const goToNote = () => view.goTo?.(href);
      const resolved = await view.book?.resolveHref?.(href);
      if (request !== latest) return;
      if (!resolved || resolved.index < 0) {
        goToNote();
        return;
      }
      const shown = view.renderer
        ?.getContents?.()
        .find((contents) => contents.index === resolved.index)?.doc;
      const doc =
        shown ??
        (await view.book?.sections?.[resolved.index]?.createDocument?.());
      if (request !== latest) return;
      const target = doc ? resolved.anchor(doc) : null;
      const paragraphs = footnoteText(
        (target as Node | null)?.nodeType === Node.ELEMENT_NODE
          ? (target as Element)
          : null,
      );
      if (!paragraphs.length) {
        goToNote();
        return;
      }
      // The reference's box is in its frame's coordinates, which the frame
      // may scale (see the click handler above).
      const frame = a.ownerDocument.defaultView
        ?.frameElement as HTMLElement | null;
      const frameRect = frame?.getBoundingClientRect();
      const scale =
        frame && frameRect && frame.offsetWidth
          ? frameRect.width / frame.offsetWidth
          : 1;
      const box = a.getBoundingClientRect();
      const bookFontFamily =
        a.ownerDocument.defaultView?.getComputedStyle(a.parentElement ?? a)
          .fontFamily || "serif";
      setFootnoteOpen(true);
      setFootnote({
        href,
        paragraphs,
        bookFontFamily,
        rect: {
          left: (frameRect?.left ?? 0) + box.left * scale,
          top: (frameRect?.top ?? 0) + box.top * scale,
          width: box.width * scale,
          height: box.height * scale,
        },
      });
    };

    const handleLink = (evt: Event) => {
      const { a, href } = (evt as CustomEvent<{ a: Element; href: string }>)
        .detail;
      if (!isFootnoteReference(a)) return;
      evt.preventDefault();
      showFootnote(a, href).catch((err: unknown) => {
        console.warn("ReflowableReader: couldn't show the footnote", err);
        view.goTo?.(href);
      });
    };
    view.addEventListener("link", handleLink);
    return () => {
      latest++;
      view.removeEventListener("link", handleLink);
    };
  }, [viewElement]);

  // Wire the relocate event for progress tracking.
  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;

    const handleRelocate = (evt: Event) => {
      const detail = (evt as CustomEvent<RelocateDetail>).detail;
      if (!detail) return;
      // An open footnote points at a spot on the page it came from, which a
      // move or a re-layout leaves.
      setFootnoteOpen(false);
      if (typeof detail.fraction === "number") setFraction(detail.fraction);
      setCurrentTocHref(detail.tocItem?.href ?? null);
      setCurrentTocLabel(detail.tocItem?.label ?? null);
      // A drag that doesn't reach the next page snaps back to where it was, and
      // the location foliate then reports can differ from a saved one taken at
      // another screen size.
      const renderer = (view as HTMLElement & { renderer?: FoliateRenderer })
        .renderer;
      const place = screenOf(detail.section?.current, renderer);
      const samePlace = place === placeRef.current;
      placeRef.current = place;
      if (detail.reason === "snap" && samePlace) return;
      // Opening at the saved location and re-layouts aren't moves.
      if (restoringRef.current || detail.reason === "anchor" || !detail.cfi) {
        return;
      }
      const foliate = view as HTMLElement & {
        renderer?: FoliateRenderer;
        book?: { sections?: Array<{ linear?: string }> };
      };
      // A nudge of the wheel on the first screen isn't reading yet.
      if (
        detail.reason === "scroll" &&
        detail.section?.current === openedSectionRef.current &&
        withinFirstScreen(foliate.renderer)
      ) {
        return;
      }
      moved({
        cfi: detail.cfi,
        percentage: detail.fraction ?? 0,
        screen: place,
        atEnd: readerAtEnd(
          foliate.renderer,
          detail.section?.current,
          foliate.book?.sections,
        ),
      });
    };

    view.addEventListener("relocate", handleRelocate);
    return () => view.removeEventListener("relocate", handleRelocate);
  }, [bookReady, moved]);

  // Push the display settings to foliate. foliate re-lays out the section
  // and returns to the same place, which it reports as an "anchor" relocate.
  useEffect(() => {
    if (!bookReady || fontPending) return;
    const view = viewRef.current as
      (HTMLElement & { renderer?: FoliateRenderer }) | null;
    const renderer = view?.renderer;
    if (!renderer) return;

    renderer.setStyles?.(readerCSS(readerSettings, fontFace));
    const layout = readerLayout(readerSettings);
    // Setting an attribute lays the book out again even when it's unchanged.
    const set = (name: string, value: string) => {
      if (renderer.getAttribute?.(name) !== value) {
        renderer.setAttribute?.(name, value);
      }
    };
    set("gap", layout.gap);
    set("flow", layout.flow);
    set("max-column-count", layout.maxColumnCount);

    // A section already on screen lays out before a newly chosen font
    // arrives, and Firefox doesn't notice the text change size (see
    // paginator.js `load`), so lay it out again once the font is in.
    let stale = false;
    const family = fontFace && `1em "${fontFace.family}"`;
    for (const { doc } of family ? (renderer.getContents?.() ?? []) : []) {
      if (!family || doc.fonts?.check?.(family) !== false) continue;
      doc.fonts
        .load(family)
        .then(() => {
          if (!stale) renderer.render?.();
        })
        .catch(() => {
          // The page keeps the fallback font it already shows.
        });
    }
    return () => {
      stale = true;
    };
  }, [bookReady, fontPending, readerSettings, fontFace]);

  // The menu value for the current location. Entries are keyed by place in
  // the menu, since a book can list one href twice. foliate reports a heading
  // as the current item with no href, so a heading is found by its label.
  const currentTocIndex = currentTocHref
    ? toc.findIndex((e) => e.href === currentTocHref)
    : toc.findIndex((e) => !e.href && e.label === currentTocLabel);
  const currentTocValue = currentTocIndex >= 0 ? String(currentTocIndex) : "";

  const handleTocChange = (value: string) => {
    const href = toc[Number(value)]?.href;
    if (!href) return;
    const view = viewRef.current as
      (HTMLElement & { goTo?: (target: string) => void }) | null;
    view?.goTo?.(href);
  };

  const handleProgressClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const target = (e.clientX - rect.left) / rect.width;
    const view = viewRef.current as
      (HTMLElement & { goToFraction?: (f: number) => void }) | null;
    view?.goToFraction?.(Math.max(0, Math.min(1, target)));
  };

  const handleProgressKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== "Home" && e.key !== "End") return;
    e.preventDefault();
    const view = viewRef.current as
      (HTMLElement & { goToFraction?: (f: number) => void }) | null;
    view?.goToFraction?.(e.key === "Home" ? 0 : 1);
  };

  // To a tenth of a percent, so the bar and the number move with each page
  // in all but the longest books.
  const progressPercent = useMemo(
    () => Math.round(fraction * 1000) / 10,
    [fraction],
  );

  // A failed background refetch keeps an open book on screen.
  if ((error && !blob) || loadError) {
    // foliate's loadError is client-side and says what went wrong in the
    // reader; a request error reads through requestErrorMessage.
    const message = loadError
      ? loadError.message
      : requestErrorMessage(error, "Failed to load book");
    return (
      <div className="fixed inset-x-0 bottom-0 top-[var(--banner-height,0px)] bg-background flex flex-col items-center justify-center gap-4 p-4 text-center">
        <AlertCircle className="h-8 w-8 text-destructive" />
        <div>
          <p className="font-medium">We couldn't load this book.</p>
          <p className="text-sm text-muted-foreground mt-1">{message}</p>
        </div>
        <Button
          onClick={() => {
            setLoadError(null);
            // refetch() runs even a disabled query, so respect its gate.
            if (isEnabled) refetch();
          }}
          variant="default"
        >
          Retry
        </Button>
      </div>
    );
  }

  return (
    <div className="fixed inset-x-0 bottom-0 top-[var(--banner-height,0px)] bg-background flex flex-col">
      <header
        className={cn(
          "flex items-center justify-end px-4 py-2 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60",
          hideChrome &&
            "fixed top-[var(--banner-height,0px)] inset-x-0 z-20 transition-transform duration-300",
          hideChrome && !chromeVisible && "-translate-y-full",
        )}
        ref={headerRef}
      >
        <div className="flex items-center gap-2">
          {toc.length > 0 && (
            <ReaderChapterSelect
              entries={toc.map((entry, index) => ({
                value: String(index),
                label: entry.label,
                disabled: !entry.href,
                depth: entry.depth,
              }))}
              onValueChange={handleTocChange}
              value={currentTocValue}
            />
          )}
          <Popover onOpenChange={setSettingsOpen} open={settingsOpen}>
            <PopoverTrigger asChild>
              <Button aria-label="Settings" size="icon" variant="ghost">
                <Settings className="h-4 w-4" />
              </Button>
            </PopoverTrigger>
            <PopoverContent
              align="end"
              className="w-80 max-h-[var(--radix-popover-content-available-height)] overflow-y-auto"
            >
              <ReaderSettingsPanel
                importedFonts={importedFonts}
                onChange={updateReaderSettings}
                onImportFont={importFont}
                onRemoveFont={removeFont}
                settings={readerSettings}
              >
                <div className="flex items-center justify-between">
                  <Label htmlFor="hide-chrome">Auto-hide controls</Label>
                  <Switch
                    checked={hideChrome}
                    disabled={!settingsReady}
                    id="hide-chrome"
                    onCheckedChange={(checked) =>
                      commitSettings({ viewer_hide_chrome: checked })
                    }
                  />
                </div>
              </ReaderSettingsPanel>
            </PopoverContent>
          </Popover>
        </div>
      </header>

      <main
        className={cn(
          "relative bg-background",
          hideChrome
            ? "fixed inset-x-0 bottom-0 top-[var(--banner-height,0px)]"
            : "flex-1",
        )}
      >
        {(isLoading || !blob || !bookReady) && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-background z-20">
            <LoadingSpinner />
            <p className="text-sm text-muted-foreground">Preparing book…</p>
            {showExtendedHint && (
              <p className="text-xs text-muted-foreground">
                This may take a moment for large books.
              </p>
            )}
          </div>
        )}

        {/* Tapping the page toggles the controls; keyboard users get a button
            that shows on focus and doesn't otherwise cover the book. */}
        {hideChrome && flow !== "scrolled" && (
          <Button
            className="sr-only focus-visible:not-sr-only focus-visible:fixed focus-visible:bottom-24 focus-visible:left-1/2 focus-visible:z-30 focus-visible:-translate-x-1/2"
            onClick={toggleChrome}
            variant="secondary"
          >
            Toggle controls
          </Button>
        )}

        <foliate-view
          ref={attachView}
          style={{
            display: "block",
            position: "absolute",
            inset: 0,
          }}
        />

        <Popover
          onOpenChange={(open) => {
            if (!open) closeFootnote();
          }}
          open={footnoteOpen}
        >
          {footnote && (
            <PopoverAnchor asChild>
              <div
                aria-hidden="true"
                className="pointer-events-none fixed"
                style={footnote.rect}
              />
            </PopoverAnchor>
          )}
          <PopoverContent
            aria-label="Footnote"
            className="w-80 max-w-[calc(100vw-2rem)] max-h-[min(50vh,var(--radix-popover-content-available-height))] overflow-y-auto"
            // Keeps the 16px page gutter on a phone, where a reference near
            // the edge would otherwise push the popover against it.
            collisionPadding={16}
            // The reader theme's colors and type, not the app's: they come
            // from the reader settings, which no Tailwind token holds.
            style={
              noteStyle
                ? {
                    backgroundColor: noteStyle.backgroundColor,
                    color: noteStyle.color,
                    borderColor:
                      "color-mix(in srgb, currentColor 25%, transparent)",
                    fontFamily: noteStyle.fontFamily,
                    fontSize: noteStyle.fontSize,
                    lineHeight: noteStyle.lineHeight,
                  }
                : undefined
            }
          >
            {/* The chosen font's faces, which this page loads only while a
                note shows. */}
            {noteFontFace && <style>{noteFontFace.faces}</style>}
            <div className="space-y-2">
              {footnote?.paragraphs.map((text, i) => (
                <p key={i}>{text}</p>
              ))}
            </div>
            <Button
              className="mt-2"
              onClick={() => {
                if (!footnote) return;
                const view = viewRef.current as FoliateBookView | null;
                view?.goTo?.(footnote.href);
                closeFootnote();
              }}
              size="inline"
              style={noteStyle ? { color: noteStyle.link } : undefined}
              variant="link"
            >
              Go to note
            </Button>
          </PopoverContent>
        </Popover>
      </main>

      <footer
        className={cn(
          "border-t bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60",
          hideChrome &&
            "fixed bottom-0 inset-x-0 z-20 transition-transform duration-300",
          hideChrome && !chromeVisible && "translate-y-full",
        )}
        ref={footerRef}
      >
        {/* The window's arrow-key handler already turns pages, so the bar
            adds only Home and End for keyboard seeking. */}
        <div className="px-4 pt-3">
          <div
            aria-label="Reading progress"
            aria-valuemax={100}
            aria-valuemin={0}
            aria-valuenow={progressPercent}
            aria-valuetext={`${progressPercent.toFixed(1)}%`}
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
            <ChapterMarkers
              markers={chapterMarks.map((mark) => ({
                position: mark.position,
                label: mark.label,
                // The chapter's own link, so a click lands on its first
                // page rather than wherever the fraction falls.
                onSelect: () =>
                  (viewRef.current as FoliateBookView | null)?.goTo?.(
                    mark.href,
                  ),
              }))}
            />
          </div>
          {currentTocLabel && (
            <div className="text-xs text-muted-foreground mt-1">
              {currentTocLabel}
            </div>
          )}
        </div>

        <div className="flex items-center justify-between px-4 py-2">
          <Button
            aria-label={rtl ? "Next page" : "Previous page"}
            disabled={!bookReady}
            onClick={goPrev}
            size="icon"
            variant="ghost"
          >
            <ChevronLeft className="h-5 w-5" />
          </Button>
          <span className="text-sm text-muted-foreground tabular-nums">
            {progressPercent.toFixed(1)}%
          </span>
          <Button
            aria-label={rtl ? "Previous page" : "Next page"}
            disabled={!bookReady}
            onClick={goNext}
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
