import { ChevronDown, ChevronUp } from "lucide-react";
import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type RefObject,
} from "react";
import { Link, useLocation } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { useFilePageUrl } from "@/hooks/useFilePageUrl";
import { FileTypePDF, type File } from "@/types";
import { formatCount } from "@/utils/format";

// The grid's column rule: tiles at least this wide, this far apart.
const TILE_MIN_PX = 96;
const GAP_PX = 12;
const COLLAPSED_ROWS = 2;

const FADE = "linear-gradient(to bottom, black 60%, transparent 100%)";

// The history entries whose grid was opened, so Back from the reader finds
// it still open. In memory on purpose: a reload starts it closed.
const expandedEntries = new Set<string>();

// Each tile shows its page rendered whole. The server renders a PDF's pages
// one at a time, so a PDF loads one tile at a time and opening the reader
// never waits behind a queue of renders; a CBZ page is only unpacked.
const loadsAtOnce = (file: File) => (file.file_type === FileTypePDF ? 1 : 4);

/**
 * Which tiles are on screen (or near it), so their pages can load as the
 * reader scrolls to them. `null` without IntersectionObserver (jsdom), where
 * every tile counts as on screen.
 */
const useTilesOnScreen = (gridRef: RefObject<HTMLElement | null>) => {
  const [visible, setVisible] = useState<ReadonlySet<number> | null>(
    typeof IntersectionObserver === "undefined" ? null : new Set(),
  );

  useEffect(() => {
    const grid = gridRef.current;
    if (!grid || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(
      (entries) =>
        setVisible((prev) => {
          const next = new Set(prev);
          for (const entry of entries) {
            const page = Number((entry.target as HTMLElement).dataset.page);
            if (entry.isIntersecting) next.add(page);
            else next.delete(page);
          }
          return next;
        }),
      { rootMargin: "200px" },
    );
    const observeTiles = () => {
      for (const tile of grid.querySelectorAll("[data-page]")) {
        observer.observe(tile);
      }
    };
    observeTiles();
    // More columns and Show all add tiles; watch those too.
    const mutations = new MutationObserver(observeTiles);
    mutations.observe(grid, { childList: true });
    return () => {
      observer.disconnect();
      mutations.disconnect();
    };
  }, [gridRef]);

  return visible;
};

/**
 * A PDF's or CBZ's pages as a grid of tiles, each opening the reader on that
 * page. It opens two rows tall, the second fading out above Show all; Show
 * all lays out every page, since asking for them is a choice to see them
 * all.
 */
const FilePageGrid = ({ file }: { file: File }) => {
  const pageUrl = useFilePageUrl();
  // A history entry and the file it showed; one entry shows only one file.
  const entry = `${useLocation().key}:${file.id}`;
  const [expanded, setExpandedState] = useState(() =>
    expandedEntries.has(entry),
  );
  const gridRef = useRef<HTMLDivElement>(null);
  const [columns, setColumns] = useState(1);
  const onScreen = useTilesOnScreen(gridRef);
  const [loaded, setLoaded] = useState<ReadonlySet<number>>(new Set());
  const count = file.page_count ?? 0;

  // How many tiles fit across, so the collapsed grid renders exactly its
  // two rows: nothing clipped, and nothing hidden for the keyboard to reach.
  useLayoutEffect(() => {
    const el = gridRef.current;
    if (!el) return;
    const measure = () =>
      setColumns(
        Math.max(
          1,
          Math.floor((el.clientWidth + GAP_PX) / (TILE_MIN_PX + GAP_PX)),
        ),
      );
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, [count]);

  if (count === 0) return null;

  const setExpanded = (next: boolean) => {
    if (next) expandedEntries.add(entry);
    else expandedEntries.delete(entry);
    setExpandedState(next);
  };
  const collapsedTiles = columns * COLLAPSED_ROWS;
  const overflows = count > collapsedTiles;
  const shown = expanded ? count : Math.min(count, collapsedTiles);
  const readerUrl = `/libraries/${file.library_id}/books/${file.book_id}/files/${file.id}/read`;

  // The lowest pages on screen still to load, as many as may load at once,
  // in page order. A tile keeps its image once it has one.
  const loading = Array.from({ length: shown }, (_, page) => page)
    .filter((page) => (onScreen ? onScreen.has(page) : true))
    .filter((page) => !loaded.has(page))
    .slice(0, loadsAtOnce(file));
  const mayLoad = (page: number) => loaded.has(page) || loading.includes(page);
  const finished = (page: number) =>
    setLoaded((prev) => (prev.has(page) ? prev : new Set(prev).add(page)));

  return (
    <section aria-labelledby="file-pages-heading">
      <h2
        className="mb-3 text-sm text-muted-foreground"
        id="file-pages-heading"
      >
        {formatCount(count, "page")}
      </h2>
      <div
        className="grid gap-3"
        id="file-pages-grid"
        ref={gridRef}
        style={{
          gridTemplateColumns: `repeat(auto-fill, minmax(${TILE_MIN_PX}px, 1fr))`,
          ...(!expanded &&
            overflows && { maskImage: FADE, WebkitMaskImage: FADE }),
        }}
      >
        {Array.from({ length: shown }, (_, page) => {
          // Pages are 0-indexed in storage and the reader's URL, 1-indexed
          // on screen.
          const number = (page + 1).toLocaleString();
          return (
            <Link
              aria-label={`Page ${number}`}
              className="group block"
              data-page={page}
              key={page}
              to={`${readerUrl}?page=${page}`}
            >
              <span className="block aspect-[2/3] overflow-hidden rounded border border-border bg-muted">
                {mayLoad(page) && (
                  <img
                    alt=""
                    className="h-full w-full bg-white object-contain transition-opacity group-hover:opacity-80"
                    onError={() => finished(page)}
                    onLoad={() => finished(page)}
                    src={pageUrl(file, page)}
                  />
                )}
              </span>
              <span className="mt-1 block text-center text-xs tabular-nums text-muted-foreground">
                {number}
              </span>
            </Link>
          );
        })}
      </div>
      {(overflows || expanded) && (
        <Button
          aria-controls="file-pages-grid"
          aria-expanded={expanded}
          className="mt-1 gap-1 font-medium"
          onClick={() => setExpanded(!expanded)}
          variant="link"
        >
          {expanded ? "Show fewer" : `Show all ${formatCount(count, "page")}`}
          {expanded ? (
            <ChevronUp aria-hidden="true" className="size-4" />
          ) : (
            <ChevronDown aria-hidden="true" className="size-4" />
          )}
        </Button>
      )}
    </section>
  );
};

export default FilePageGrid;
