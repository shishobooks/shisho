import { useParams } from "react-router-dom";

import BookDetailBody from "@/components/library/BookDetailBody";
import LibraryBreadcrumbs from "@/components/library/LibraryBreadcrumbs";
import LibraryLayout from "@/components/library/LibraryLayout";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import { useBook } from "@/hooks/queries/books";
import { useUserLibrary } from "@/hooks/queries/libraries";
import { usePageTitle } from "@/hooks/usePageTitle";
import { isLoadFailure } from "@/libraries/api";

const BookDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const bookQuery = useBook(id);
  const libraryQuery = useUserLibrary(libraryId);

  usePageTitle(bookQuery.data?.title ?? "Book Details");

  if (bookQuery.isLoading) {
    return (
      <LibraryLayout>
        <LoadingSpinner />
      </LibraryLayout>
    );
  }

  if (isLoadFailure(bookQuery)) {
    return (
      <LibraryLayout>
        <QueryError fallback="Failed to load book" query={bookQuery} />
      </LibraryLayout>
    );
  }

  if (!bookQuery.data) {
    return (
      <LibraryLayout>
        <div className="text-center">
          <h1 className="text-2xl font-semibold mb-4">Book Not Found</h1>
          <p className="text-muted-foreground">
            The book you're looking for doesn't exist or may have been removed.
          </p>
        </div>
      </LibraryLayout>
    );
  }

  const book = bookQuery.data;

  return (
    <LibraryLayout>
      <LibraryBreadcrumbs
        items={[{ label: book.title }]}
        libraryId={libraryId!}
        libraryName={libraryQuery.data?.name ?? book.library?.name}
      />
      <BookDetailBody book={book} library={libraryQuery.data} />
    </LibraryLayout>
  );
};

export default BookDetail;
