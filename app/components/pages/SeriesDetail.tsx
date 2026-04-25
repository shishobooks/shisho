import { Edit, GitMerge, Trash2 } from "lucide-react";
import { useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";

import BookItem from "@/components/library/BookItem";
import LibraryBreadcrumbs from "@/components/library/LibraryBreadcrumbs";
import LibraryLayout from "@/components/library/LibraryLayout";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { MetadataDeleteDialog } from "@/components/library/MetadataDeleteDialog";
import { MetadataEditDialog } from "@/components/library/MetadataEditDialog";
import { MetadataMergeDialog } from "@/components/library/MetadataMergeDialog";
import { SizeButton, SizePopover } from "@/components/library/SizePopover";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DEFAULT_GALLERY_SIZE } from "@/constants/gallerySize";
import { useLibrary } from "@/hooks/queries/libraries";
import {
  useDeleteSeries,
  useMergeSeries,
  useSeries,
  useSeriesBooks,
  useSeriesList,
  useUpdateSeries,
} from "@/hooks/queries/series";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { useDebounce } from "@/hooks/useDebounce";
import { usePageTitle } from "@/hooks/usePageTitle";
import { parseGallerySize } from "@/libraries/gallerySize";
import type { GallerySize } from "@/types";

const SeriesDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const seriesId = id ? parseInt(id, 10) : undefined;

  const userSettingsQuery = useUserSettings();
  const updateUserSettings = useUpdateUserSettings();

  const urlSize: GallerySize | null = parseGallerySize(
    searchParams.get("size"),
  );
  const savedSize: GallerySize =
    userSettingsQuery.data?.gallery_size ?? DEFAULT_GALLERY_SIZE;
  const effectiveSize: GallerySize = urlSize ?? savedSize;
  const isSizeDirty = urlSize !== null && urlSize !== savedSize;

  const userSettingsResolved =
    userSettingsQuery.isSuccess || userSettingsQuery.isError;

  // No page recalc here — this gallery is unpaginated (single unbounded
  // fetch). The paginated pages (Home / SeriesList / ListDetail) call
  // pageForSizeChange to preserve the user's first-visible book across
  // size changes; that math is meaningless when there's only one page.
  const applyGallerySize = (next: GallerySize) => {
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      if (next === savedSize) {
        params.delete("size");
      } else {
        params.set("size", next);
      }
      return params;
    });
  };

  const handleSaveSizeAsDefault = () => {
    updateUserSettings.mutate(
      { gallery_size: effectiveSize },
      {
        onSuccess: () => {
          setSearchParams((prev) => {
            const params = new URLSearchParams(prev);
            params.delete("size");
            return params;
          });
        },
      },
    );
  };

  const libraryQuery = useLibrary(libraryId);
  const seriesQuery = useSeries(seriesId);

  usePageTitle(seriesQuery.data?.name ?? "Series");
  const seriesBooksQuery = useSeriesBooks(seriesId, {
    enabled: userSettingsResolved && Boolean(seriesId),
  });

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
      <LibraryLayout>
        <LoadingSpinner />
      </LibraryLayout>
    );
  }

  if (!seriesQuery.isSuccess || !seriesQuery.data) {
    return (
      <LibraryLayout>
        <div className="text-center">
          <h1 className="text-2xl font-semibold mb-4">Series Not Found</h1>
          <p className="text-muted-foreground">
            The series you're looking for doesn't exist or may have been
            removed.
          </p>
        </div>
      </LibraryLayout>
    );
  }

  const series = seriesQuery.data;
  const canDelete = series.book_count === 0;

  return (
    <LibraryLayout>
      <LibraryBreadcrumbs
        items={[
          { label: "Series", to: `/libraries/${libraryId}/series` },
          { label: series.name },
        ]}
        libraryId={libraryId!}
        libraryName={libraryQuery.data?.name}
      />

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
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-xl font-semibold">Books in Series</h2>
            <div className="hidden sm:flex">
              <SizePopover
                effectiveSize={effectiveSize}
                isSaving={updateUserSettings.isPending}
                onChange={applyGallerySize}
                onSaveAsDefault={handleSaveSizeAsDefault}
                savedSize={savedSize}
                trigger={<SizeButton isDirty={isSizeDirty} />}
              />
            </div>
          </div>
          {seriesBooksQuery.isLoading && <LoadingSpinner />}
          {seriesBooksQuery.isSuccess && (
            <div className="flex flex-wrap gap-4">
              {seriesBooksQuery.data.map((book) => (
                <BookItem
                  book={book}
                  cacheKey={seriesBooksQuery.dataUpdatedAt}
                  gallerySize={effectiveSize}
                  key={book.id}
                  libraryId={libraryId!}
                  seriesId={seriesId}
                />
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
        sortNameSource={series.sort_name_source}
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
    </LibraryLayout>
  );
};

export default SeriesDetail;
