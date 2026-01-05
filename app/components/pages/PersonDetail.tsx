import { Link, useParams } from "react-router-dom";

import BookItem from "@/components/library/BookItem";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import {
  usePerson,
  usePersonAuthoredBooks,
  usePersonNarratedFiles,
} from "@/hooks/queries/people";

const PersonDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const personId = id ? parseInt(id, 10) : undefined;

  const personQuery = usePerson(personId);
  const authoredBooksQuery = usePersonAuthoredBooks(personId);
  const narratedFilesQuery = usePersonNarratedFiles(personId);

  if (personQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (!personQuery.isSuccess || !personQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <div className="text-center">
            <h1 className="text-2xl font-semibold mb-4">Person Not Found</h1>
            <p className="text-muted-foreground mb-6">
              The person you're looking for doesn't exist or may have been
              removed.
            </p>
            <Link
              className="text-primary hover:underline"
              to={`/libraries/${libraryId}/people`}
            >
              Back to People
            </Link>
          </div>
        </div>
      </div>
    );
  }

  const person = personQuery.data;

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        {/* Person Header */}
        <div className="mb-8">
          <div className="flex items-center gap-4 mb-2">
            <h1 className="text-3xl font-bold">{person.name}</h1>
          </div>
          {person.sort_name !== person.name && (
            <p className="text-muted-foreground mb-2">
              Sort name: {person.sort_name}
            </p>
          )}
          <div className="flex gap-2">
            {person.authored_book_count > 0 && (
              <Badge variant="secondary">
                {person.authored_book_count} book
                {person.authored_book_count !== 1 ? "s" : ""} authored
              </Badge>
            )}
            {person.narrated_file_count > 0 && (
              <Badge variant="outline">
                {person.narrated_file_count} file
                {person.narrated_file_count !== 1 ? "s" : ""} narrated
              </Badge>
            )}
          </div>
        </div>

        {/* Authored Books Section */}
        {person.authored_book_count > 0 && (
          <section className="mb-10">
            <h2 className="text-xl font-semibold mb-4">Books Authored</h2>
            {authoredBooksQuery.isLoading && <LoadingSpinner />}
            {authoredBooksQuery.isSuccess && (
              <div className="flex flex-wrap gap-6">
                {authoredBooksQuery.data.map((book) => (
                  <BookItem book={book} key={book.id} libraryId={libraryId!} />
                ))}
              </div>
            )}
          </section>
        )}

        {/* Narrated Files Section */}
        {person.narrated_file_count > 0 && (
          <section className="mb-10">
            <h2 className="text-xl font-semibold mb-4">Files Narrated</h2>
            {narratedFilesQuery.isLoading && <LoadingSpinner />}
            {narratedFilesQuery.isSuccess && (
              <div className="space-y-2">
                {narratedFilesQuery.data.map((file) => (
                  <Link
                    className="flex items-center justify-between p-4 rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
                    key={file.id}
                    to={`/libraries/${libraryId}/books/${file.book_id}`}
                  >
                    <div className="flex-1">
                      <div className="font-medium">
                        {file.book?.title ?? "Unknown Book"}
                      </div>
                      <div className="text-sm text-muted-foreground">
                        {file.file_type.toUpperCase()} -{" "}
                        {file.audiobook_duration_seconds
                          ? `${Math.round(file.audiobook_duration_seconds / 60)} min`
                          : "Duration unknown"}
                      </div>
                    </div>
                    <Badge variant="outline">{file.file_type}</Badge>
                  </Link>
                ))}
              </div>
            )}
          </section>
        )}

        {/* No Works */}
        {person.authored_book_count === 0 &&
          person.narrated_file_count === 0 && (
            <div className="text-center py-8 text-muted-foreground">
              This person has no associated books or files.
            </div>
          )}
      </div>
    </div>
  );
};

export default PersonDetail;
