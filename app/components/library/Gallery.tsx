import { ReactNode } from "react";
import { useSearchParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import PaginationFooter, {
  type PageChangeHandler,
} from "@/components/library/PaginationFooter";
import QueryError, {
  type RetryableQuery,
} from "@/components/library/QueryError";
import { parsePageParam } from "@/libraries/pagination";
import { formatShowingRange } from "@/utils/format";

interface GalleryProps<T> {
  items: T[];
  total: number;
  /** True while the shown items are not the current ones (loading, refetching, stale). */
  isLoading: boolean;
  /** The query behind the items, for the error report and its Retry. */
  query: RetryableQuery & { data: unknown; isPlaceholderData: boolean };
  itemsPerPage?: number;
  renderItem: (item: T) => ReactNode;
  itemLabel: string;
  emptyMessage?: string;
}

const Gallery = <T,>({
  items,
  total,
  isLoading,
  query,
  itemsPerPage = 20,
  renderItem,
  itemLabel,
  emptyMessage,
}: GalleryProps<T>) => {
  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parsePageParam(searchParams.get("page"));

  const limit = itemsPerPage;
  const offset = (currentPage - 1) * limit;
  const totalPages = Math.ceil(total / itemsPerPage);
  const showing = formatShowingRange(offset, limit, total, itemLabel);

  const handlePageChange: PageChangeHandler = (page, options) => {
    const newSearchParams = new URLSearchParams(searchParams);
    newSearchParams.set("page", page.toString());
    setSearchParams(newSearchParams, options);
  };

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

      <div className="flex flex-wrap gap-4 sm:gap-4 mb-6 md:mb-8">
        {items.map(renderItem)}
      </div>

      <PaginationFooter
        className="mb-8"
        currentPage={currentPage}
        onPageChange={handlePageChange}
        totalIsCurrent={!query.isPlaceholderData}
        totalPages={totalPages}
      />
    </>
  );
};

export default Gallery;
