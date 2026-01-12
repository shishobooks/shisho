import { Edit, GitMerge, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import BookItem from "@/components/library/BookItem";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { MetadataDeleteDialog } from "@/components/library/MetadataDeleteDialog";
import { MetadataEditDialog } from "@/components/library/MetadataEditDialog";
import { MetadataMergeDialog } from "@/components/library/MetadataMergeDialog";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  useDeleteGenre,
  useGenre,
  useGenreBooks,
  useGenresList,
  useMergeGenre,
  useUpdateGenre,
} from "@/hooks/queries/genres";
import { useDebounce } from "@/hooks/useDebounce";

const GenreDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const genreId = id ? parseInt(id, 10) : undefined;

  const genreQuery = useGenre(genreId);
  const genreBooksQuery = useGenreBooks(genreId);

  const [editOpen, setEditOpen] = useState(false);
  const [mergeOpen, setMergeOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [mergeSearch, setMergeSearch] = useState("");
  const debouncedMergeSearch = useDebounce(mergeSearch, 200);

  const updateGenreMutation = useUpdateGenre();
  const mergeGenreMutation = useMergeGenre();
  const deleteGenreMutation = useDeleteGenre();

  const genresListQuery = useGenresList(
    {
      library_id: genreQuery.data?.library_id,
      limit: 50,
      search: debouncedMergeSearch || undefined,
    },
    { enabled: mergeOpen && !!genreQuery.data?.library_id },
  );

  const handleEdit = async (data: { name: string }) => {
    if (!genreId) return;
    await updateGenreMutation.mutateAsync({
      genreId,
      payload: { name: data.name },
    });
    setEditOpen(false);
  };

  const handleMerge = async (sourceId: number) => {
    if (!genreId) return;
    await mergeGenreMutation.mutateAsync({
      targetId: genreId,
      sourceId,
    });
    setMergeOpen(false);
  };

  const handleDelete = async () => {
    if (!genreId) return;
    await deleteGenreMutation.mutateAsync({ genreId });
    setDeleteOpen(false);
    navigate(`/libraries/${libraryId}/genres`);
  };

  if (genreQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (!genreQuery.isSuccess || !genreQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <div className="text-center">
            <h1 className="text-2xl font-semibold mb-4">Genre Not Found</h1>
            <p className="text-muted-foreground mb-6">
              The genre you're looking for doesn't exist or may have been
              removed.
            </p>
            <Link
              className="text-primary hover:underline"
              to={`/libraries/${libraryId}/genres`}
            >
              Back to Genres
            </Link>
          </div>
        </div>
      </div>
    );
  }

  const genre = genreQuery.data;
  const bookCount = genre.book_count ?? 0;
  const canDelete = bookCount === 0;

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        {/* Genre Header */}
        <div className="mb-8">
          <div className="flex items-start justify-between gap-4 mb-2">
            <h1 className="text-3xl font-bold min-w-0 break-words">
              {genre.name}
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
          <Badge variant="secondary">
            {bookCount} book{bookCount !== 1 ? "s" : ""}
          </Badge>
        </div>

        {/* Books with this Genre */}
        {bookCount > 0 && (
          <section className="mb-10">
            <h2 className="text-xl font-semibold mb-4">Books</h2>
            {genreBooksQuery.isLoading && <LoadingSpinner />}
            {genreBooksQuery.isSuccess && (
              <div className="flex flex-wrap gap-6">
                {genreBooksQuery.data.map((book) => (
                  <BookItem book={book} key={book.id} libraryId={libraryId!} />
                ))}
              </div>
            )}
          </section>
        )}

        {/* No Books */}
        {bookCount === 0 && (
          <div className="text-center py-8 text-muted-foreground">
            This genre has no associated books.
          </div>
        )}

        <MetadataEditDialog
          entityName={genre.name}
          entityType="genre"
          isPending={updateGenreMutation.isPending}
          onOpenChange={setEditOpen}
          onSave={handleEdit}
          open={editOpen}
        />

        <MetadataMergeDialog
          entities={
            genresListQuery.data?.genres.map((g) => ({
              id: g.id,
              name: g.name,
              count: g.book_count ?? 0,
            })) ?? []
          }
          entityType="genre"
          isLoadingEntities={genresListQuery.isLoading}
          isPending={mergeGenreMutation.isPending}
          onMerge={handleMerge}
          onOpenChange={setMergeOpen}
          onSearch={setMergeSearch}
          open={mergeOpen}
          targetId={genreId!}
          targetName={genre.name}
        />

        <MetadataDeleteDialog
          entityName={genre.name}
          entityType="genre"
          isPending={deleteGenreMutation.isPending}
          onDelete={handleDelete}
          onOpenChange={setDeleteOpen}
          open={deleteOpen}
        />
      </div>
    </div>
  );
};

export default GenreDetail;
