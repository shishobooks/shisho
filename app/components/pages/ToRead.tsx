import { useEffect } from "react";

import BookItem from "@/components/library/BookItem";
import { DraggableBookList } from "@/components/library/DraggableBookList";
import Gallery from "@/components/library/Gallery";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import { SizeButton, SizePopover } from "@/components/library/SizePopover";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { GALLERY_MAX_PAGE_SIZE } from "@/constants/gallerySize";
import { ContentWidth } from "@/contexts/ContentWidth";
import { useBooks } from "@/hooks/queries/books";
import { useUserLibraries } from "@/hooks/queries/libraries";
import { useReorderToRead } from "@/hooks/queries/reading";
import { useAuth } from "@/hooks/useAuth";
import { useGallerySizeParam } from "@/hooks/useGallerySizeParam";
import { usePageTitle } from "@/hooks/usePageTitle";
import { DEMO_MODE_MESSAGE, toastRequestError } from "@/libraries/api";
import type { Book } from "@/types";
import { formatCount } from "@/utils/format";

const EMPTY_MESSAGE =
  "Nothing on To Read yet. Add books from a book's reading status or a book card's menu.";

// The signed-in user's To Read, across every library they can reach, in
// queue order. Reordering needs the whole queue on screen, so a queue that
// fits in one API page loads whole and can be dragged; a longer one pages
// like a gallery. In Demo Mode it never drags.
const ToRead = () => {
  usePageTitle("To Read");
  const { demoMode } = useAuth();

  const {
    savedSize,
    effectiveSize,
    isSizeDirty,
    columns,
    itemsPerPage,
    currentPage,
    offset,
    goToPage,
    pageSizeResolved,
    isSaving: isSavingSize,
    applyGallerySize,
    saveSizeAsDefault,
  } = useGallerySizeParam();

  // The whole queue up to the API's page limit, which decides whether the
  // page can drag.
  const wholeQuery = useBooks({
    to_read: true,
    limit: GALLERY_MAX_PAGE_SIZE,
    offset: 0,
  });
  const total = wholeQuery.data?.total ?? 0;
  const canReorder =
    !demoMode && Boolean(wholeQuery.data) && total <= GALLERY_MAX_PAGE_SIZE;
  const pageQuery = useBooks(
    { to_read: true, limit: itemsPerPage, offset },
    { enabled: pageSizeResolved && Boolean(wholeQuery.data) && !canReorder },
  );

  // The whole queue shows no pages, so a page position left in the URL would
  // only come back into effect if the queue outgrew the limit.
  useEffect(() => {
    if (canReorder && offset > 0) goToPage(1, { replace: true });
  }, [canReorder, offset, goToPage]);

  // To Read spans libraries; each book's cover takes the shape its own
  // library's gallery gives it.
  const userLibrariesQuery = useUserLibraries();
  const coverAspectRatioFor = (libraryId: number) =>
    userLibrariesQuery.data?.find((library) => library.id === libraryId)
      ?.cover_aspect_ratio ?? "book";

  const reorderMutation = useReorderToRead();
  const handleReorder = async (bookIds: number[]) => {
    try {
      await reorderMutation.mutateAsync({ book_ids: bookIds });
    } catch (error) {
      toastRequestError(error, "Failed to reorder To Read");
      throw error; // DraggableBookList restores the previous order
    }
  };

  const renderBooks = () => {
    if (wholeQuery.error && !wholeQuery.data) {
      return (
        <QueryError fallback="Failed to load To Read" query={wholeQuery} />
      );
    }
    if (!wholeQuery.data) {
      return <LoadingSpinner />;
    }
    if (total === 0) {
      return (
        <div className="text-center py-8 text-muted-foreground">
          {EMPTY_MESSAGE}
        </div>
      );
    }
    if (canReorder) {
      return (
        <DraggableBookList
          books={wholeQuery.data.items}
          columns={columns}
          coverAspectRatioFor={coverAspectRatioFor}
          onReorder={handleReorder}
        />
      );
    }
    return (
      <Gallery
        columns={columns}
        currentPage={currentPage}
        emptyMessage={EMPTY_MESSAGE}
        isLoading={pageQuery.isLoading}
        itemLabel="books"
        items={pageQuery.data?.items ?? []}
        itemsPerPage={itemsPerPage}
        offset={offset}
        onPageChange={goToPage}
        query={pageQuery}
        renderItem={(book: Book) => (
          <BookItem
            book={book}
            coverAspectRatio={coverAspectRatioFor(book.library_id)}
            key={book.id}
            libraryId={book.library_id.toString()}
          />
        )}
        total={pageQuery.data?.total ?? total}
      />
    );
  };

  return (
    <div>
      <TopNav />
      <ContentWidth className="px-4 py-4 md:px-6 md:py-8">
        <div className="mb-6 md:mb-8">
          <div className="flex items-center justify-between gap-4 mb-2">
            <h1 className="text-2xl font-semibold">
              To Read
              {canReorder && total > 1 && (
                <span className="text-sm font-normal text-muted-foreground ml-2">
                  (drag to reorder)
                </span>
              )}
            </h1>
            <div className="hidden sm:flex">
              <SizePopover
                effectiveSize={effectiveSize}
                isSaving={isSavingSize}
                onChange={applyGallerySize}
                onSaveAsDefault={saveSizeAsDefault}
                savedSize={savedSize}
                trigger={<SizeButton isDirty={isSizeDirty} />}
              />
            </div>
          </div>
          <p className="text-sm md:text-base text-muted-foreground mb-2">
            What you plan to read next. Starting or finishing a book takes it
            off.
          </p>
          {wholeQuery.data && (
            <Badge variant="secondary">{formatCount(total, "book")}</Badge>
          )}
          {demoMode && (
            <p className="text-xs text-muted-foreground mt-2">
              {DEMO_MODE_MESSAGE}
            </p>
          )}
        </div>
        {renderBooks()}
      </ContentWidth>
    </div>
  );
};

export default ToRead;
