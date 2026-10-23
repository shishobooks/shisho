import BookItem from "@/components/library/BookItem";
import GalleryGrid from "@/components/library/GalleryGrid";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import PaginationFooter from "@/components/library/PaginationFooter";
import QueryError, {
  type RetryableQuery,
} from "@/components/library/QueryError";
import { SizeButton, SizePopover } from "@/components/library/SizePopover";
import SortedByChips from "@/components/library/SortedByChips";
import SortSheet, { SortButton } from "@/components/library/SortSheet";
import { useUserLibrary } from "@/hooks/queries/libraries";
import { useGallerySizeParam } from "@/hooks/useGallerySizeParam";
import { SECTION_CLASS, useSectionPaging } from "@/hooks/useSectionPaging";
import type { SortParam } from "@/hooks/useSortParam";
import { firstFailedQuery } from "@/libraries/api";
import type { Book, ResourceListResponse } from "@/types";
import { formatShowingRange } from "@/utils/format";

interface BookGalleryQuery extends RetryableQuery {
  data: ResourceListResponse<Book> | undefined;
  isSuccess: boolean;
  isError: boolean;
  isPlaceholderData: boolean;
}

interface BookGallerySectionProps<F extends string> {
  libraryId: string;
  query: BookGalleryQuery;
  title: string;
  emptyMessage?: string;
  /** Series context forwarded to BookItem so it shows the series number */
  seriesId?: number;
  /** Adds the Sort control; the page passes its param to the query. */
  sort?: SortParam<F>;
}

export function BookGallerySection<F extends string>({
  libraryId,
  query,
  title,
  emptyMessage,
  seriesId,
  sort,
}: BookGallerySectionProps<F>) {
  const {
    savedSize,
    effectiveSize,
    isSizeDirty,
    columns,
    itemsPerPage,
    currentPage,
    offset,
    isSaving,
    goToPage,
    applyGallerySize,
    saveSizeAsDefault,
  } = useGallerySizeParam();
  const { sectionRef, sectionPageChange } =
    useSectionPaging<HTMLElement>(goToPage);
  // Covers take the shape the library's gallery gives them.
  const library = useUserLibrary(libraryId);
  const coverAspectRatio = library.data?.cover_aspect_ratio ?? "book";
  const totalPages = Math.ceil((query.data?.total ?? 0) / itemsPerPage);

  // Treat any unresolved query as loading, including a disabled query
  // waiting on its `enabled` gate (isLoading is false there, since TanStack
  // only sets it while actually fetching). Without this, a series/genre/tag
  // with books briefly flashes the empty state before the query is enabled.
  if (!query.isSuccess && !query.isError) {
    return (
      <section className={SECTION_CLASS} ref={sectionRef}>
        <h2 className="text-xl font-semibold mb-4">{title}</h2>
        <LoadingSpinner />
      </section>
    );
  }

  if (firstFailedQuery(query)) {
    return (
      <section className={SECTION_CLASS} ref={sectionRef}>
        <h2 className="text-xl font-semibold mb-4">{title}</h2>
        <QueryError fallback="Failed to load books" query={query} />
      </section>
    );
  }

  const total = query.data?.total ?? 0;
  const showing = formatShowingRange(offset, itemsPerPage, total, "books");

  if (total === 0) {
    return (
      <section className={SECTION_CLASS} ref={sectionRef}>
        <h2 className="text-xl font-semibold mb-4">{title}</h2>
        <div className="text-center py-8 text-muted-foreground">
          {emptyMessage ?? "No books found."}
        </div>
      </section>
    );
  }

  return (
    <section className={SECTION_CLASS} ref={sectionRef}>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-xl font-semibold">{title}</h2>
        <div className="flex items-center gap-2">
          {sort && (
            <SortSheet
              emptyLabel={sort.config.emptyLabel}
              fieldLabels={sort.config.labels}
              fields={sort.config.fields}
              isDirty={sort.isDirty}
              levels={sort.levels}
              onChange={sort.apply}
              trigger={<SortButton isDirty={sort.isDirty} />}
            />
          )}
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
      </div>

      {sort && (
        <SortedByChips
          fieldLabels={sort.config.labels}
          levels={sort.isDirty ? sort.levels : []}
          onRemoveLevel={(index) =>
            sort.apply(sort.levels.filter((_, i) => i !== index))
          }
          onReset={sort.reset}
        />
      )}

      {showing && (
        <div className="mb-4 text-sm text-muted-foreground">{showing}</div>
      )}

      <GalleryGrid columns={columns}>
        {query.data?.items.map((book) => (
          <BookItem
            book={book}
            coverAspectRatio={coverAspectRatio}
            key={book.id}
            libraryId={libraryId}
            seriesId={seriesId}
          />
        ))}
      </GalleryGrid>

      <PaginationFooter
        className="mb-8"
        currentPage={currentPage}
        onPageChange={sectionPageChange}
        totalIsCurrent={!query.isPlaceholderData}
        totalPages={totalPages}
      />
    </section>
  );
}
