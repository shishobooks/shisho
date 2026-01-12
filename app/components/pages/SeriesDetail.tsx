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
  useDeleteSeries,
  useMergeSeries,
  useSeries,
  useSeriesBooks,
  useSeriesList,
  useUpdateSeries,
} from "@/hooks/queries/series";
import { useDebounce } from "@/hooks/useDebounce";

const SeriesDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const seriesId = id ? parseInt(id, 10) : undefined;

  const seriesQuery = useSeries(seriesId);
  const seriesBooksQuery = useSeriesBooks(seriesId);

  const [editOpen, setEditOpen] = useState(false);
  const [mergeOpen, setMergeOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [mergeSearch, setMergeSearch] = useState("");
  const debouncedMergeSearch = useDebounce(mergeSearch, 200);

  const updateSeriesMutation = useUpdateSeries();
  const mergeSeriesMutation = useMergeSeries();
  const deleteSeriesMutation = useDeleteSeries();

  const seriesListQuery = useSeriesList(
    {
      library_id: seriesQuery.data?.library_id,
      limit: 50,
      search: debouncedMergeSearch || undefined,
    },
    { enabled: mergeOpen && !!seriesQuery.data?.library_id },
  );

  const handleEdit = async (data: { name: string; sort_name?: string }) => {
    if (!seriesId) return;
    await updateSeriesMutation.mutateAsync({
      seriesId,
      payload: {
        name: data.name,
        sort_name: data.sort_name,
      },
    });
    setEditOpen(false);
  };

  const handleMerge = async (sourceId: number) => {
    if (!seriesId) return;
    await mergeSeriesMutation.mutateAsync({
      targetId: seriesId,
      sourceId,
    });
    setMergeOpen(false);
  };

  const handleDelete = async () => {
    if (!seriesId) return;
    await deleteSeriesMutation.mutateAsync({ seriesId });
    setDeleteOpen(false);
    navigate(`/libraries/${libraryId}/series`);
  };

  if (seriesQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (!seriesQuery.isSuccess || !seriesQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <div className="text-center">
            <h1 className="text-2xl font-semibold mb-4">Series Not Found</h1>
            <p className="text-muted-foreground mb-6">
              The series you're looking for doesn't exist or may have been
              removed.
            </p>
            <Link
              className="text-primary hover:underline"
              to={`/libraries/${libraryId}/series`}
            >
              Back to Series
            </Link>
          </div>
        </div>
      </div>
    );
  }

  const series = seriesQuery.data;
  const canDelete = series.book_count === 0;

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        {/* Series Header */}
        <div className="mb-8">
          <div className="flex items-start justify-between gap-4 mb-2">
            <h1 className="text-3xl font-bold min-w-0 break-words">
              {series.name}
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
          {series.sort_name !== series.name && (
            <p className="text-muted-foreground mb-2">
              Sort name: {series.sort_name}
            </p>
          )}
          {series.description && (
            <p className="text-muted-foreground mb-2">{series.description}</p>
          )}
          <Badge variant="secondary">
            {series.book_count} book{series.book_count !== 1 ? "s" : ""}
          </Badge>
        </div>

        {/* Books in Series */}
        {series.book_count > 0 && (
          <section className="mb-10">
            <h2 className="text-xl font-semibold mb-4">Books in Series</h2>
            {seriesBooksQuery.isLoading && <LoadingSpinner />}
            {seriesBooksQuery.isSuccess && (
              <div className="flex flex-wrap gap-6">
                {seriesBooksQuery.data.map((book) => (
                  <BookItem book={book} key={book.id} libraryId={libraryId!} />
                ))}
              </div>
            )}
          </section>
        )}

        {/* No Books */}
        {series.book_count === 0 && (
          <div className="text-center py-8 text-muted-foreground">
            This series has no associated books.
          </div>
        )}

        <MetadataEditDialog
          entityName={series.name}
          entityType="series"
          isPending={updateSeriesMutation.isPending}
          onOpenChange={setEditOpen}
          onSave={handleEdit}
          open={editOpen}
          sortName={series.sort_name}
        />

        <MetadataMergeDialog
          entities={
            seriesListQuery.data?.series.map((s) => ({
              id: s.id,
              name: s.name,
              count: s.book_count ?? 0,
            })) ?? []
          }
          entityType="series"
          isLoadingEntities={seriesListQuery.isLoading}
          isPending={mergeSeriesMutation.isPending}
          onMerge={handleMerge}
          onOpenChange={setMergeOpen}
          onSearch={setMergeSearch}
          open={mergeOpen}
          targetId={seriesId!}
          targetName={series.name}
        />

        <MetadataDeleteDialog
          entityName={series.name}
          entityType="series"
          isPending={deleteSeriesMutation.isPending}
          onDelete={handleDelete}
          onOpenChange={setDeleteOpen}
          open={deleteOpen}
        />
      </div>
    </div>
  );
};

export default SeriesDetail;
