import { useSearchParams } from "react-router-dom";

import BookItem from "@/components/library/BookItem";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import PaginationFooter from "@/components/library/PaginationFooter";
import QueryError, {
  type RetryableQuery,
} from "@/components/library/QueryError";
import { SizeButton, SizePopover } from "@/components/library/SizePopover";
import { useGallerySizeParam } from "@/hooks/useGallerySizeParam";
import { firstFailedQuery } from "@/libraries/api";
import type { Book, GallerySize, ResourceListResponse } from "@/types";

interface BookGalleryQuery extends RetryableQuery {
  data: ResourceListResponse<Book> | undefined;
  isSuccess: boolean;
  isError: boolean;
}

interface BookGallerySectionProps {
  libraryId: string;
  query: BookGalleryQuery;
  title: string;
  emptyMessage?: string;
  /** Series context forwarded to BookItem so it shows the series number */
  seriesId?: number;
  /** Called when pagination page changes */
  onPageChange?: (page: number) => void;
  /** Called when gallery size changes */
  onSizeChange?: (size: GallerySize) => void;
}

export function BookGallerySection({
  libraryId,
  query,
  title,
  emptyMessage,
  seriesId,
  onPageChange,
  onSizeChange,
}: BookGallerySectionProps) {
  const [, setSearchParams] = useSearchParams();
  const {
    savedSize,
    effectiveSize,
    isSizeDirty,
    itemsPerPage,
    currentPage,
    offset,
    isSaving,
    applyGallerySize,
    saveSizeAsDefault,
  } = useGallerySizeParam({
    onChange: (size, page) => {
      onSizeChange?.(size);
      onPageChange?.(page);
    },
  });
  const totalPages = Math.ceil((query.data?.total ?? 0) / itemsPerPage);

  const handlePageChange = (page: number) => {
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      if (page === 1) {
        params.delete("page");
      } else {
        params.set("page", page.toString());
      }
      return params;
    });
    onPageChange?.(page);
  };

  // Treat any unresolved query as loading, including a disabled query
  // waiting on its `enabled` gate (isLoading is false there, since TanStack
  // only sets it while actually fetching). Without this, a series/genre/tag
  // with books briefly flashes the empty state before the query is enabled.
  if (!query.isSuccess && !query.isError) {
    return (
      <section className="mb-10">
        <h2 className="text-xl font-semibold mb-4">{title}</h2>
        <LoadingSpinner />
      </section>
    );
  }

  if (firstFailedQuery(query)) {
    return (
      <section className="mb-10">
        <h2 className="text-xl font-semibold mb-4">{title}</h2>
        <QueryError fallback="Failed to load books" query={query} />
      </section>
    );
  }

  const total = query.data?.total ?? 0;

  if (total === 0) {
    return (
      <section className="mb-10">
        <h2 className="text-xl font-semibold mb-4">{title}</h2>
        <div className="text-center py-8 text-muted-foreground">
          {emptyMessage ?? "No books found."}
        </div>
      </section>
    );
  }

  return (
    <section className="mb-10">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-xl font-semibold">{title}</h2>
        <div className="hidden sm:flex">
          <SizePopover
            effectiveSize={effectiveSize}
            isSaving={isSaving}
            onChange={applyGallerySize}
            onSaveAsDefault={saveSizeAsDefault}
            savedSize={savedSize}
            trigger={<SizeButton isDirty={isSizeDirty} />}
          />
        </div>
      </div>

      <div className="mb-4 text-sm text-muted-foreground">
        Showing {offset + 1}-{Math.min(offset + itemsPerPage, total)} of {total}{" "}
        books
      </div>

      <div className="flex flex-wrap gap-4 mb-6 md:mb-8">
        {query.data?.items.map((book) => (
          <BookItem
            book={book}
            gallerySize={effectiveSize}
            key={book.id}
            libraryId={libraryId}
            seriesId={seriesId}
          />
        ))}
      </div>

      <PaginationFooter
        className="mb-8"
        currentPage={currentPage}
        onPageChange={handlePageChange}
        totalPages={totalPages}
      />
    </section>
  );
}
