import { useParams } from "react-router-dom";

import BookDetailBody from "@/components/library/BookDetailBody";
import LibraryBreadcrumbs from "@/components/library/LibraryBreadcrumbs";
import LibraryLayout from "@/components/library/LibraryLayout";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { useBook } from "@/hooks/queries/books";
import { useLibrary } from "@/hooks/queries/libraries";
import { useAuth } from "@/hooks/useAuth";
import { usePageTitle } from "@/hooks/usePageTitle";

const BookDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const bookQuery = useBook(id);
  const { hasPermission } = useAuth();
  const libraryQuery = useLibrary(libraryId, {
    enabled: Boolean(libraryId) && hasPermission("libraries", "read"),
  });

  usePageTitle(bookQuery.data?.title ?? "Book Details");

  if (bookQuery.isLoading) {
    return (
      <LibraryLayout>
        <LoadingSpinner />
      </LibraryLayout>
    );
  }

  if (!bookQuery.isSuccess || !bookQuery.data) {
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
