import { ReactNode } from "react";

import GalleryGrid from "@/components/library/GalleryGrid";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import PaginationFooter, {
  type PageChangeHandler,
} from "@/components/library/PaginationFooter";
import QueryError, {
  type RetryableQuery,
} from "@/components/library/QueryError";
import { formatShowingRange } from "@/utils/format";

interface GalleryProps<T> {
  items: T[];
  total: number;
  /** True while the shown items are not the current ones (first load, another key's placeholder, a pending search). */
  isLoading: boolean;
  /** The query behind the items, for the error report and its Retry. */
  query: RetryableQuery & { data: unknown; isPlaceholderData: boolean };
  /** Paging from `useGallerySizeParam`. */
  columns: number;
  itemsPerPage: number;
  offset: number;
  currentPage: number;
  onPageChange: PageChangeHandler;
  renderItem: (item: T) => ReactNode;
  itemLabel: string;
  emptyMessage?: string;
}

const Gallery = <T,>({
  items,
  total,
  isLoading,
  query,
  columns,
  itemsPerPage,
  offset,
  currentPage,
  onPageChange,
  renderItem,
  itemLabel,
  emptyMessage,
}: GalleryProps<T>) => {
  const totalPages = Math.ceil(total / itemsPerPage);
  const showing = formatShowingRange(offset, itemsPerPage, total, itemLabel);

  // Only shown items count, so the line says nothing while they load.
  const showsItems = !isLoading && Boolean(query.data);

  // One live region that stays mounted while the results load and change, so
  // a new count or the empty state is announced. A region mounted with its
  // text is often not.
  const status = (
    <div role="status">
      {showsItems && showing && (
        <div className="mb-4 text-sm text-muted-foreground">{showing}</div>
      )}
      {showsItems && total === 0 && (
        <div className="text-center py-8 text-muted-foreground">
          {emptyMessage ?? `No ${itemLabel} found.`}
        </div>
      )}
    </div>
  );

  if (isLoading) {
    return (
      <>
        {status}
        <LoadingSpinner />
      </>
    );
  }

  if (query.error && !query.data) {
    return (
      <>
        {status}
        <QueryError fallback={`Failed to load ${itemLabel}`} query={query} />
      </>
    );
  }

  // A query waiting on its enabled gate has neither data nor an error.
  if (!query.data) {
    return (
      <>
        {status}
        <LoadingSpinner />
      </>
    );
  }

  return (
    <>
      {status}

      <GalleryGrid columns={columns}>{items.map(renderItem)}</GalleryGrid>

      <PaginationFooter
        className="mb-8"
        currentPage={currentPage}
        onPageChange={onPageChange}
        totalIsCurrent={!query.isPlaceholderData}
        totalPages={totalPages}
      />
    </>
  );
};

export default Gallery;
