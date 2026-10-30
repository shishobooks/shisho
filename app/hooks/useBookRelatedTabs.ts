import type { RetryableQuery } from "@/components/library/QueryError";
import { useBooks } from "@/hooks/queries/books";
import { useBookDevices } from "@/hooks/queries/devices";
import { useBookLists } from "@/hooks/queries/lists";
import { useBookReading } from "@/hooks/queries/reading";
import { useCan } from "@/hooks/useCan";
import type { Book, BookSeries, List } from "@/types";
import { deviceHasWarning } from "@/utils/devices";

// The search param that keeps the Related panel's open tab, so it survives
// a reload and carries along as the reader clicks from book to book. A
// search param rather than the route's tab segment: the tabs are a panel on
// the page, not the page's own sections.
export const RELATED_TAB_PARAM = "related";

const RELATED_TAB_KEYS = ["series", "author", "lists", "devices"] as const;
type RelatedTabKey = (typeof RELATED_TAB_KEYS)[number];

/** The tab a `?related=` value names, or undefined for any other value. */
export const relatedTabKey = (
  value: string | null,
): RelatedTabKey | undefined => RELATED_TAB_KEYS.find((key) => key === value);

// The most of an author's books the Author tab lists.
const MAX_AUTHOR_BOOKS = 100;

export interface RelatedTab {
  key: RelatedTabKey;
  label: string;
  count?: number;
  warning?: boolean;
}

export interface BookRelatedTabs {
  /** The tabs with something to show, in display order. */
  tabs: RelatedTab[];
  series: BookSeries[];
  author?: {
    personId: number;
    name: string;
    books: Book[];
    total: number;
    /** Set when the books failed to load; `books` is then empty. */
    failed?: RetryableQuery;
  };
  lists: List[];
  /** Set when the lists failed to load; `lists` is then empty. */
  listsFailed?: RetryableQuery;
  /** Whether the book is on the reader's To Read, which leads the Lists tab. */
  onToRead: boolean;
  linkPeople: boolean;
}

/**
 * Loads what the book page's Related panel shows and decides its tabs, so
 * the page can leave the panel out when no tab has anything. A tab appears
 * only with content: the series (Series Read), the first author's books
 * when they have another, the lists the book is on (and To Read, which
 * leads them), and the reader's Devices. `enabled: false` (a Share Link page) sends no request, even for
 * a viewer who is signed in.
 */
export const useBookRelatedTabs = (
  book: Book,
  { enabled = true }: { enabled?: boolean } = {},
): BookRelatedTabs => {
  const canReadSeries = useCan("series:read");
  const linkPeople = useCan("people:read");

  const firstAuthor = book.authors?.[0];
  const authorBooks = useBooks(
    {
      library_id: book.library_id,
      person_id: firstAuthor?.person_id,
      limit: MAX_AUTHOR_BOOKS,
    },
    { enabled: enabled && Boolean(firstAuthor?.person) },
  );
  const lists = useBookLists(book.id, { enabled });
  const reading = useBookReading(book.id, { enabled });
  const devices = useBookDevices(book, { enabled });

  if (!enabled) {
    return { tabs: [], series: [], lists: [], onToRead: false, linkPeople };
  }
  const series = canReadSeries ? (book.book_series ?? []) : [];
  const authorItems = authorBooks.data?.items ?? [];
  // A failure to load shows in its tab, which reports it with a Retry, so a
  // tab that may have content does not vanish over a failed request.
  const authorFailed =
    authorBooks.error && !authorBooks.data ? authorBooks : undefined;
  const author =
    firstAuthor?.person &&
    (authorFailed || authorItems.some((b) => b.id !== book.id))
      ? {
          personId: firstAuthor.person_id,
          name: firstAuthor.person.name,
          books: authorItems,
          total: authorBooks.data?.total ?? authorItems.length,
          failed: authorFailed,
        }
      : undefined;
  const listData = lists.data ?? [];
  const listsFailed = lists.error && !lists.data ? lists : undefined;

  const tabs: RelatedTab[] = [];
  if (series.length > 0) tabs.push({ key: "series", label: "Series" });
  if (author) tabs.push({ key: "author", label: "Author" });
  const onToRead = Boolean(reading.data?.on_to_read);
  if (listData.length > 0 || listsFailed || onToRead) {
    tabs.push({
      key: "lists",
      label: "Lists",
      count: lists.data && lists.data.length + (onToRead ? 1 : 0),
    });
  }
  if ((devices.data?.length ?? 0) > 0 || (devices.error && !devices.data)) {
    tabs.push({
      key: "devices",
      label: "Devices",
      count: devices.data?.length,
      warning: devices.data?.some(deviceHasWarning),
    });
  }

  return {
    tabs,
    series,
    author,
    lists: listData,
    listsFailed,
    onToRead,
    linkPeople,
  };
};
