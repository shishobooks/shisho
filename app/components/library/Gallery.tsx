import { ReactNode } from "react";
import { useSearchParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import PaginationFooter from "@/components/library/PaginationFooter";
import QueryError, {
  type RetryableQuery,
} from "@/components/library/QueryError";
import { parsePageParam } from "@/libraries/pagination";

interface GalleryProps<T> {
  items: T[];
  total: number;
  /** True while the shown items are not the current ones (loading, refetching, stale). */
  isLoading: boolean;
  /** The query behind the items, for the error report and its Retry. */
  query: RetryableQuery & { data: unknown };
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

  const handlePageChange = (page: number) => {
    const newSearchParams = new URLSearchParams(searchParams);
    newSearchParams.set("page", page.toString());
    setSearchParams(newSearchParams);
  };

  if (isLoading) {
    return <LoadingSpinner />;
  }

  if (query.error && !query.data) {
    return (
      <QueryError fallback={`Failed to load ${itemLabel}`} query={query} />
    );
  }

  // A query waiting on its enabled gate has neither data nor an error.
  if (!query.data) {
    return <LoadingSpinner />;
  }

  return (
    <>
      {total > 0 && (
        <div className="mb-4 text-sm text-muted-foreground">
          Showing {offset + 1}-{Math.min(offset + limit, total)} of {total}{" "}
          {itemLabel}
        </div>
      )}

      {total === 0 && (
        <div className="text-center py-8 text-muted-foreground">
          {emptyMessage ?? `No ${itemLabel} found.`}
        </div>
      )}

      <div className="flex flex-wrap gap-4 sm:gap-4 mb-6 md:mb-8">
        {items.map(renderItem)}
      </div>

      <PaginationFooter
        className="mb-8"
        currentPage={currentPage}
        onPageChange={handlePageChange}
        totalPages={totalPages}
      />
    </>
  );
};

export default Gallery;
