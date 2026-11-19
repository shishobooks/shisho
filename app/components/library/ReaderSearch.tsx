import { ChevronDown, ChevronUp, List, Loader2, Search, X } from "lucide-react";
import {
  useEffect,
  useId,
  useImperativeHandle,
  useRef,
  useState,
  type Ref,
} from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  useBookSearch,
  type SearchableBook,
  type SearchJump,
} from "@/hooks/useBookSearch";
import { cn } from "@/libraries/utils";
import { formatCount } from "@/utils/format";

// Results the list shows at first and adds with each "Show more". A short
// query can match tens of thousands of times in a long book.
const RESULTS_PAGE = 100;

// What the reader's keyboard shortcuts do in an open search panel.
export interface ReaderSearchHandle {
  next: () => void;
  previous: () => void;
  // Puts the keys in the field, its query selected to type over.
  focusField: () => void;
}

interface ReaderSearchProps {
  // The open book, or null until it's ready.
  view: SearchableBook | null;
  // Makes each move to a result, so the reader can count it in its history
  // of jumps. The results of one search share a group.
  onJump: SearchJump;
  // Gives the keys to the book, so the arrow keys turn its pages.
  onFocusBook: () => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // Set while the panel is open.
  handleRef?: Ref<ReaderSearchHandle>;
}

/**
 * Search inside the Book, from a button in the reader's header. The panel
 * stays open while the reader turns pages, like a browser's find bar, so it
 * closes only from its own controls, Escape, or the button that opened it.
 */
export default function ReaderSearch({
  view,
  onJump,
  onFocusBook,
  handleRef,
  open,
  onOpenChange,
}: ReaderSearchProps) {
  return (
    <Popover onOpenChange={onOpenChange} open={open}>
      <PopoverTrigger asChild>
        <Button
          aria-label="Search"
          disabled={!view}
          size="icon"
          variant="ghost"
        >
          <Search className="h-4 w-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        aria-label="Search in book"
        className="flex w-96 max-w-[calc(100vw-2rem)] max-h-[min(70vh,var(--radix-popover-content-available-height))] flex-col p-0"
        // Keeps the 16px page gutter on a phone.
        collisionPadding={16}
        // Turning pages and tapping the book leave the search open.
        onInteractOutside={(e) => e.preventDefault()}
      >
        {view && (
          <SearchPanel
            handleRef={handleRef}
            onClose={() => onOpenChange(false)}
            onFocusBook={onFocusBook}
            onJump={onJump}
            view={view}
          />
        )}
      </PopoverContent>
    </Popover>
  );
}

// Mounted only while the panel is open, so closing it ends the search and
// clears its highlights.
function SearchPanel({
  view,
  onClose,
  onJump,
  onFocusBook,
  handleRef,
}: {
  view: SearchableBook;
  onClose: () => void;
  onJump: SearchJump;
  onFocusBook: () => void;
  handleRef?: Ref<ReaderSearchHandle>;
}) {
  const search = useBookSearch(view, onJump);
  const { query, results, groups, searching, waiting, current } = search;
  const fieldRef = useRef<HTMLInputElement>(null);
  // The list folds away once a result is chosen, so it doesn't cover the
  // page it went to; the field and the arrows stay.
  const [listOpen, setListOpen] = useState(true);
  const [shownCount, setShownCount] = useState(RESULTS_PAGE);
  const listId = useId();
  const listRef = useRef<HTMLDivElement>(null);

  // Opening the list again brings the chosen result into view.
  useEffect(() => {
    if (!listOpen) return;
    listRef.current
      ?.querySelector('[aria-current="true"]')
      ?.scrollIntoView({ block: "nearest" });
  }, [listOpen]);

  const total = results.length;
  const compact = !listOpen && current !== null;
  // A result stepped to beyond the shown ones extends the list to it.
  const shown = Math.max(shownCount, (current ?? -1) + 1);

  let statusText = "";
  if (query.trim()) {
    if (current !== null) {
      statusText = `${(current + 1).toLocaleString()} of ${total.toLocaleString()}`;
    } else if (total) {
      statusText = formatCount(total, "result");
    } else if (waiting) {
      statusText = "Press Enter to search";
    } else {
      statusText = searching ? "Searching…" : "No results";
    }
  }
  // Said beside a count. The single row has room only for the spinner, so
  // there it's for screen readers alone.
  const stillSearching = searching && (current !== null || total > 0);

  // Going to a result gives the keys to the book, so the arrow keys turn
  // its pages from there. Enter in the field keeps them, to step again.
  const choose = (index: number) => {
    setListOpen(false);
    onFocusBook();
    search.goToResult(index);
  };

  // A new query shows its results from the top.
  const changeQuery = (next: string) => {
    setListOpen(true);
    setShownCount(RESULTS_PAGE);
    search.changeQuery(next);
  };

  const stepTo = (direction: "next" | "previous") => {
    setListOpen(false);
    if (direction === "next") search.next();
    else search.previous();
  };

  // Stepping by shortcut leaves the keys where they are, so a reader in the
  // book can go on turning pages.
  useImperativeHandle(handleRef, () => ({
    next: () => stepTo("next"),
    previous: () => stepTo("previous"),
    focusField: () => {
      fieldRef.current?.focus();
      fieldRef.current?.select();
    },
  }));

  return (
    <>
      {/* One set of controls in two arrangements: the field over the count
          and arrows, or, with a result chosen and the list folded, a single
          row like a browser's find bar, so the panel covers as little of the
          page as it can. */}
      {/* The single row is slim enough to end above the 48px foliate keeps
          above a page's text, so it never covers the first line. */}
      <div
        className={cn(
          "flex flex-wrap items-center gap-1",
          compact ? "px-3 py-1.5" : "p-3",
        )}
      >
        <div
          className={cn(
            "relative order-1 min-w-0",
            compact ? "flex-1" : "basis-[calc(100%-2.5rem)]",
          )}
        >
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            aria-label="Search in book"
            // The browser's own clear button doesn't follow the app's colors,
            // and Firefox has none, so the field gets its own.
            className={cn(
              "pl-8 pr-8 [&::-webkit-search-cancel-button]:hidden",
              compact && "h-8",
            )}
            enterKeyHint="search"
            onChange={(e) => changeQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key !== "Enter") return;
              // Enter that confirms an input method's text isn't a step.
              // Safari reports it with keyCode 229 and isComposing false.
              if (e.nativeEvent.isComposing || e.keyCode === 229) return;
              e.preventDefault();
              stepTo(e.shiftKey ? "previous" : "next");
            }}
            placeholder="Search"
            ref={fieldRef}
            type="search"
            value={query}
          />
          {query && (
            <Button
              aria-label="Clear search"
              className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground"
              onClick={() => {
                changeQuery("");
                fieldRef.current?.focus();
              }}
              size="icon-xs"
              variant="ghost"
            >
              <X className="h-4 w-4" />
            </Button>
          )}
        </div>
        <p
          className={cn(
            "flex items-center gap-2 text-sm text-muted-foreground tabular-nums",
            compact ? "order-2 shrink-0 px-1" : "order-3 flex-1",
            statusText && "min-h-7",
          )}
          role="status"
        >
          {searching && (
            <Loader2 aria-hidden="true" className="h-3.5 w-3.5 animate-spin" />
          )}
          {/* One run of text, or the row's gap would space out the comma. */}
          <span>
            {stillSearching && !compact
              ? `${statusText}, still searching…`
              : statusText}
            {stillSearching && compact && (
              <span className="sr-only">, still searching…</span>
            )}
          </span>
        </p>
        {total > 0 && (
          <div
            className={cn(
              "flex items-center gap-1",
              compact ? "order-3" : "order-4",
            )}
          >
            <Button
              aria-controls={listOpen ? listId : undefined}
              aria-expanded={listOpen}
              aria-label="Results"
              onClick={() => setListOpen((v) => !v)}
              size="icon-sm"
              variant="ghost"
            >
              <List className="h-4 w-4" />
            </Button>
            <Button
              aria-label="Previous result"
              onClick={(e) => {
                // A click, not Enter or Space on the focused arrow, which
                // keeps the keys there to press it again.
                if (e.detail > 0) onFocusBook();
                stepTo("previous");
              }}
              size="icon-sm"
              variant="ghost"
            >
              <ChevronUp className="h-4 w-4" />
            </Button>
            <Button
              aria-label="Next result"
              onClick={(e) => {
                if (e.detail > 0) onFocusBook();
                stepTo("next");
              }}
              size="icon-sm"
              variant="ghost"
            >
              <ChevronDown className="h-4 w-4" />
            </Button>
          </div>
        )}
        <Button
          aria-label="Close search"
          className={compact ? "order-4" : "order-2"}
          onClick={onClose}
          size={compact ? "icon-sm" : "icon"}
          variant="ghost"
        >
          <X className="h-4 w-4" />
        </Button>
      </div>

      {listOpen && total > 0 && (
        <div
          className="min-h-0 overflow-y-auto border-t pb-2"
          id={listId}
          ref={listRef}
        >
          {groups.map((group) => {
            const visible = group.results.filter(({ index }) => index < shown);
            if (!visible.length) return null;
            return (
              <section key={group.results[0].index}>
                <h3 className="px-3 pb-1 pt-3 text-xs font-medium text-muted-foreground">
                  {group.chapter || "Untitled section"}
                </h3>
                <ul>
                  {visible.map(({ result, index }) => (
                    <li key={index}>
                      <Button
                        aria-current={index === current ? "true" : undefined}
                        className={cn(
                          "w-full px-3 py-2 text-left text-sm leading-snug",
                          index === current ? "bg-muted" : "hover:bg-muted",
                        )}
                        onClick={() => choose(index)}
                        variant="unstyled"
                      >
                        <span>
                          {result.excerpt.pre}
                          <mark className="bg-yellow-300 text-foreground dark:bg-yellow-700">
                            {result.excerpt.match}
                          </mark>
                          {result.excerpt.post}
                        </span>
                      </Button>
                    </li>
                  ))}
                </ul>
              </section>
            );
          })}
          {shown < total && (
            <Button
              className="mx-3 mt-2"
              onClick={() => setShownCount(shown + RESULTS_PAGE)}
              size="sm"
              variant="outline"
            >
              Show more results
            </Button>
          )}
        </div>
      )}
    </>
  );
}
