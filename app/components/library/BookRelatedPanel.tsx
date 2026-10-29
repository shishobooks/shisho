import { useEffect, useRef, useState, type ReactNode } from "react";
import { Link, useSearchParams } from "react-router-dom";

import BookDevicesPanel from "@/components/library/BookDevicesPanel";
import CoverImage from "@/components/library/CoverImage";
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError, {
  type RetryableQuery,
} from "@/components/library/QueryError";
import TextLink from "@/components/library/TextLink";
import TruncatedText from "@/components/library/TruncatedText";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useListBooks } from "@/hooks/queries/lists";
import { useSeriesBooks } from "@/hooks/queries/series";
import {
  RELATED_TAB_PARAM,
  relatedTabKey,
  type BookRelatedTabs,
} from "@/hooks/useBookRelatedTabs";
import { cn } from "@/libraries/utils";
import { ListSortManual, type Book, type BookSeries, type List } from "@/types";
import { getCoverFileType } from "@/utils/coverSelection";
import { bookCoverUrl } from "@/utils/coverUrl";
import { formatCount, formatYear } from "@/utils/format";
import { hasAnyCBZFile } from "@/utils/hasAnyCBZFile";
import { seriesNumberLabel } from "@/utils/seriesNumber";

// The most books a series or list tab lists. The series or list page shows
// the rest.
const MAX_BOOKS = 100;

/**
 * One book in a tab: thumbnail, a small line (its number or year), and its
 * title. Every tab that lists books lists them this way. The current book
 * is marked, not linked, and is what the panel scrolls to.
 */
const BookLine = ({
  book,
  meta,
  current,
}: {
  book: Book;
  meta?: ReactNode;
  current: boolean;
}) => {
  const [searchParams] = useSearchParams();
  // Carries the tab the reader picked, which the next book opens if it has
  // it. Only a tab's name: anything else in the param stays behind.
  const tab = relatedTabKey(searchParams.get(RELATED_TAB_PARAM));
  const coverUrl = bookCoverUrl(book);
  // Keyed by URL, so a new cover gets its own try.
  const [failedCover, setFailedCover] = useState<string>();
  const showCover = !!book.cover_cache_key && failedCover !== coverUrl;
  const row = (
    <div
      className={cn(
        "flex items-center gap-3 rounded-md px-1.5 py-1",
        current ? "bg-muted" : "hover:bg-muted/60",
      )}
      data-current={current || undefined}
    >
      <div className="flex h-12 w-8 shrink-0 overflow-hidden rounded-sm bg-muted">
        {showCover ? (
          <CoverImage
            alt=""
            className="size-full object-cover"
            key={coverUrl}
            onError={() => setFailedCover(coverUrl)}
            src={coverUrl}
          />
        ) : (
          // The frame is always book-shaped, so the library's cover aspect
          // ratio doesn't apply: the icon is headphones only for a book with
          // nothing but audiobooks.
          <CoverPlaceholder
            className="h-full"
            variant={getCoverFileType(book.files, "book")}
          />
        )}
      </div>
      <div className="min-w-0">
        {(meta || current) && (
          <span className="block text-xs text-muted-foreground">
            {meta}
            {meta && current && " · "}
            {current && "This book"}
          </span>
        )}
        <TruncatedText className="line-clamp-2 text-sm leading-snug">
          {book.title}
        </TruncatedText>
      </div>
    </div>
  );
  if (current) return <div aria-current="page">{row}</div>;
  return (
    <Link
      className="block"
      to={`/libraries/${book.library_id}/books/${book.id}${tab ? `?${RELATED_TAB_PARAM}=${tab}` : ""}`}
    >
      {row}
    </Link>
  );
};

/** A group's heading, which stays in view while its books scroll. */
const GroupHeading = ({ children }: { children: ReactNode }) => (
  <h3 className="sticky top-0 z-10 bg-background px-1.5 py-1 text-xs font-medium text-muted-foreground">
    {children}
  </h3>
);

/** A group's spinner while its books load, or its error when they fail. */
const GroupStatus = ({
  query,
  fallback,
}: {
  query: RetryableQuery & { isPending: boolean; data: unknown };
  fallback: string;
}) => {
  if (query.error && !query.data) {
    return <QueryError className="mx-1.5" fallback={fallback} query={query} />;
  }
  if (query.isPending) return <LoadingSpinner className="py-4" />;
  return null;
};

const SeriesGroup = ({
  book,
  membership,
}: {
  book: Book;
  membership: BookSeries;
}) => {
  const query = useSeriesBooks(membership.series_id, { limit: MAX_BOOKS });
  const books = query.data?.items ?? [];
  const numberOf = (b: Book) => {
    const bs = b.book_series?.find((s) => s.series_id === membership.series_id);
    if (!bs) return undefined;
    return (
      seriesNumberLabel(
        bs.series_number,
        bs.series_number_end,
        bs.series_number_unit,
        hasAnyCBZFile(b) ? "cbz" : null,
      ) || undefined
    );
  };
  return (
    <section>
      <GroupHeading>
        <TextLink
          to={`/libraries/${book.library_id}/series/${membership.series_id}`}
        >
          {membership.series?.name ?? "Unknown Series"}
        </TextLink>
        {query.data && ` · ${formatCount(query.data.total, "book")}`}
      </GroupHeading>
      <GroupStatus fallback="Failed to load the series" query={query} />
      {books.map((b) => (
        <BookLine
          book={b}
          current={b.id === book.id}
          key={b.id}
          meta={numberOf(b)}
        />
      ))}
    </section>
  );
};

// A book's release date as the gallery's release-date sort reads it: the
// newest of its files' dates, or "" when none has one.
const releaseDate = (b: Book): string =>
  (b.files ?? []).reduce(
    (newest, f) =>
      f.release_date && f.release_date > newest ? f.release_date : newest,
    "",
  );

const firstSeriesNumber = (b: Book): number =>
  b.book_series?.[0]?.series_number ?? Number.MAX_SAFE_INTEGER;

/**
 * The author's books, this one included and oldest first (undated last), so the list stays
 * put as the reader clicks through it, the way a series does. Books that
 * share a date (one publisher's reissue dates every volume alike) fall back
 * to series order, then title.
 */
const sortAuthorBooks = (books: Book[]): Book[] =>
  [...books].sort(
    (a, b) =>
      // Undated books go last, after the dated ones.
      Number(!releaseDate(a)) - Number(!releaseDate(b)) ||
      releaseDate(a).localeCompare(releaseDate(b)) ||
      firstSeriesNumber(a) - firstSeriesNumber(b) ||
      a.title.localeCompare(b.title),
  );

const AuthorGroup = ({
  book,
  author,
  linkPeople,
}: {
  book: Book;
  author: NonNullable<BookRelatedTabs["author"]>;
  linkPeople: boolean;
}) => (
  <section>
    <GroupHeading>
      <TextLink
        to={
          linkPeople
            ? `/libraries/${book.library_id}/people/${author.personId}`
            : null
        }
      >
        {author.name}
      </TextLink>
      {!author.failed && ` · ${formatCount(author.total, "book")}`}
    </GroupHeading>
    {author.failed && (
      <QueryError
        className="mx-1.5"
        fallback="Failed to load the author's books"
        query={author.failed}
      />
    )}
    {sortAuthorBooks(author.books).map((b) => {
      const date = releaseDate(b);
      return (
        <BookLine
          book={b}
          current={b.id === book.id}
          key={b.id}
          meta={date ? formatYear(date) : undefined}
        />
      );
    })}
  </section>
);

const ListGroup = ({ book, list }: { book: Book; list: List }) => {
  // An ordered list reads in its own order, whatever its default sort, and
  // numbers each book by its position rather than its row here: the list
  // can hold books from libraries this reader cannot see.
  const query = useListBooks(list.id, {
    limit: MAX_BOOKS,
    ...(list.is_ordered ? { sort: ListSortManual } : {}),
  });
  const entries = (query.data?.items ?? []).filter((lb) => lb.book);
  return (
    <section>
      <GroupHeading>
        <TextLink to={`/lists/${list.id}`}>{list.name}</TextLink>
        {query.data && ` · ${formatCount(query.data.total, "book")}`}
      </GroupHeading>
      <GroupStatus fallback="Failed to load the list" query={query} />
      {entries.map((lb) => (
        <BookLine
          book={lb.book!}
          current={lb.book_id === book.id}
          key={lb.id}
          meta={
            list.is_ordered && lb.sort_order ? `#${lb.sort_order}` : undefined
          }
        />
      ))}
    </section>
  );
};

/**
 * A fixed-height window, four and a half books tall so the cut-off book
 * says it scrolls. It keeps the current book centered while the rows load
 * (groups load one by one and push it down) and until the reader scrolls
 * it themselves. A copy hidden by CSS has no height and waits to be shown.
 */
const ScrollWindow = ({ children }: { children: ReactNode }) => {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    let touched = false;
    const center = () => {
      const row = el.querySelector<HTMLElement>("[data-current]");
      if (touched || !row || el.clientHeight === 0) return;
      el.scrollTop = row.offsetTop - el.clientHeight / 2 + row.clientHeight / 2;
    };
    const stop = () => {
      touched = true;
    };
    const mutations = new MutationObserver(center);
    const resizes = new ResizeObserver(center);
    mutations.observe(el, { childList: true, subtree: true });
    resizes.observe(el);
    const events = ["wheel", "touchstart", "pointerdown", "keydown"] as const;
    for (const event of events) el.addEventListener(event, stop);
    center();
    return () => {
      mutations.disconnect();
      resizes.disconnect();
      for (const event of events) el.removeEventListener(event, stop);
    };
  }, []);
  return (
    // 24px of heading plus 4.5 rows of 56px.
    <div className="relative h-[276px] overflow-y-auto pr-1" ref={ref}>
      {children}
    </div>
  );
};

interface BookRelatedPanelProps {
  book: Book;
  related: BookRelatedTabs;
}

/**
 * The book's neighbors, a tab for each kind: the rest of its series, its
 * author's books, the lists it is on, and what the reader's Devices get.
 * Each tab is a scrolling window opened on this book, so clicking from book
 * to book keeps the panel in place. `useBookRelatedTabs` decides which tabs
 * exist; the page leaves the panel out when none does.
 */
const BookRelatedPanel = ({ book, related }: BookRelatedPanelProps) => {
  const [searchParams, setSearchParams] = useSearchParams();
  const { tabs } = related;
  if (tabs.length === 0) return null;

  const active =
    tabs.find((t) => t.key === searchParams.get(RELATED_TAB_PARAM))?.key ??
    tabs[0].key;
  const select = (key: string) => {
    const next = new URLSearchParams(searchParams);
    // The first tab is the default, with a clean URL.
    if (key === tabs[0].key) next.delete(RELATED_TAB_PARAM);
    else next.set(RELATED_TAB_PARAM, key);
    // Switching tabs is not a new page, so the page stays where it is: at
    // the bottom of the page on a phone, where the panel sits. It replaces
    // the history entry too, so Back leaves the book instead of stepping
    // through the tabs.
    setSearchParams(next, { preventScrollReset: true, replace: true });
  };

  const body = (key: string) => {
    switch (key) {
      case "series":
        return (
          <div className="space-y-3">
            {related.series.map((bs) => (
              <SeriesGroup book={book} key={bs.id} membership={bs} />
            ))}
          </div>
        );
      case "author":
        return (
          related.author && (
            <AuthorGroup
              author={related.author}
              book={book}
              linkPeople={related.linkPeople}
            />
          )
        );
      case "lists":
        if (related.listsFailed) {
          return (
            <QueryError
              fallback="Failed to load the lists this book is on"
              query={related.listsFailed}
            />
          );
        }
        return (
          <div className="space-y-3">
            {related.lists.map((list) => (
              <ListGroup book={book} key={list.id} list={list} />
            ))}
          </div>
        );
      default:
        return <BookDevicesPanel book={book} />;
    }
  };

  return (
    <Tabs onValueChange={select} value={active}>
      <TabsList className="w-full justify-start overflow-x-auto">
        {tabs.map((tab) => (
          <TabsTrigger
            // The badge and the dot are drawn, not read: the name says it all.
            aria-label={[
              tab.count != null ? `${tab.label} ${tab.count}` : tab.label,
              tab.warning && "a Device's next sync changes this book",
            ]
              .filter(Boolean)
              .join(", ")}
            key={tab.key}
            value={tab.key}
          >
            {tab.label}
            {tab.count != null && (
              <span
                aria-hidden="true"
                className="ml-1.5 rounded-full bg-muted px-1.5 py-px text-[10px] font-semibold tabular-nums text-muted-foreground"
              >
                {tab.count}
              </span>
            )}
            {tab.warning && (
              <span
                aria-hidden="true"
                className="ml-1.5 size-1.5 rounded-full bg-amber-500 dark:bg-amber-400"
              />
            )}
          </TabsTrigger>
        ))}
      </TabsList>
      {tabs.map((tab) => (
        <TabsContent className="mt-3" key={tab.key} value={tab.key}>
          <ScrollWindow>{body(tab.key)}</ScrollWindow>
        </TabsContent>
      ))}
    </Tabs>
  );
};

export default BookRelatedPanel;
