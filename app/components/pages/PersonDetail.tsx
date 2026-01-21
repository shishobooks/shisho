import { Edit, GitMerge, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import BookItem from "@/components/library/BookItem";
import LibraryLayout from "@/components/library/LibraryLayout";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { MetadataDeleteDialog } from "@/components/library/MetadataDeleteDialog";
import { MetadataEditDialog } from "@/components/library/MetadataEditDialog";
import { MetadataMergeDialog } from "@/components/library/MetadataMergeDialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  useDeletePerson,
  useMergePerson,
  usePeopleList,
  usePerson,
  usePersonAuthoredBooks,
  usePersonNarratedFiles,
  useUpdatePerson,
} from "@/hooks/queries/people";
import { useDebounce } from "@/hooks/useDebounce";

const PersonDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const personId = id ? parseInt(id, 10) : undefined;

  const navigate = useNavigate();

  const personQuery = usePerson(personId);
  const authoredBooksQuery = usePersonAuthoredBooks(personId);
  const narratedFilesQuery = usePersonNarratedFiles(personId);

  const [editOpen, setEditOpen] = useState(false);
  const [mergeOpen, setMergeOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [mergeSearch, setMergeSearch] = useState("");
  const debouncedMergeSearch = useDebounce(mergeSearch, 200);

  const updatePersonMutation = useUpdatePerson();
  const mergePersonMutation = useMergePerson();
  const deletePersonMutation = useDeletePerson();

  const peopleListQuery = usePeopleList(
    {
      library_id: personQuery.data?.library_id,
      limit: 50,
      search: debouncedMergeSearch || undefined,
    },
    { enabled: mergeOpen && !!personQuery.data?.library_id },
  );

  const handleEdit = async (data: { name: string; sort_name?: string }) => {
    if (!personId) return;
    await updatePersonMutation.mutateAsync({
      personId,
      payload: {
        name: data.name,
        sort_name: data.sort_name,
      },
    });
    setEditOpen(false);
  };

  const handleMerge = async (sourceId: number) => {
    if (!personId) return;
    await mergePersonMutation.mutateAsync({
      targetId: personId,
      sourceId,
    });
    setMergeOpen(false);
  };

  const handleDelete = async () => {
    if (!personId) return;
    await deletePersonMutation.mutateAsync({ personId });
    setDeleteOpen(false);
    navigate(`/libraries/${libraryId}/people`);
  };

  if (personQuery.isLoading) {
    return (
      <LibraryLayout>
        <LoadingSpinner />
      </LibraryLayout>
    );
  }

  if (!personQuery.isSuccess || !personQuery.data) {
    return (
      <LibraryLayout>
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
      </LibraryLayout>
    );
  }

  const person = personQuery.data;
  const canDelete =
    person.authored_book_count === 0 && person.narrated_file_count === 0;

  return (
    <LibraryLayout>
      {/* Person Header */}
      <div className="mb-8">
        <div className="flex items-start justify-between gap-4 mb-2">
          <h1 className="text-3xl font-bold min-w-0 break-words">
            {person.name}
          </h1>
          <div className="flex gap-2 shrink-0">
            <Button
              onClick={() => setEditOpen(true)}
              size="sm"
              variant="outline"
            >
              <Edit className="h-4 w-4 mr-2" />
              Edit
            </Button>
            <Button
              onClick={() => setMergeOpen(true)}
              size="sm"
              variant="outline"
            >
              <GitMerge className="h-4 w-4 mr-2" />
              Merge
            </Button>
            {canDelete && (
              <Button
                onClick={() => setDeleteOpen(true)}
                size="sm"
                variant="outline"
              >
                <Trash2 className="h-4 w-4 mr-2" />
                Delete
              </Button>
            )}
          </div>
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
      {person.authored_book_count === 0 && person.narrated_file_count === 0 && (
        <div className="text-center py-8 text-muted-foreground">
          This person has no associated books or files.
        </div>
      )}

      <MetadataEditDialog
        entityName={person.name}
        entityType="person"
        isPending={updatePersonMutation.isPending}
        onOpenChange={setEditOpen}
        onSave={handleEdit}
        open={editOpen}
        sortName={person.sort_name}
        sortNameSource={person.sort_name_source}
      />

      <MetadataMergeDialog
        entities={
          peopleListQuery.data?.people.map((p) => ({
            id: p.id,
            name: p.name,
            count: p.authored_book_count + p.narrated_file_count,
          })) ?? []
        }
        entityType="person"
        isLoadingEntities={peopleListQuery.isLoading}
        isPending={mergePersonMutation.isPending}
        onMerge={handleMerge}
        onOpenChange={setMergeOpen}
        onSearch={setMergeSearch}
        open={mergeOpen}
        targetId={personId!}
        targetName={person.name}
      />

      <MetadataDeleteDialog
        entityName={person.name}
        entityType="person"
        isPending={deletePersonMutation.isPending}
        onDelete={handleDelete}
        onOpenChange={setDeleteOpen}
        open={deleteOpen}
      />
    </LibraryLayout>
  );
};

export default PersonDetail;
