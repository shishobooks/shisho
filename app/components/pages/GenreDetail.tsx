import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { BookGallerySection } from "@/components/library/BookGallerySection";
import { ResourceDetail } from "@/components/library/ResourceDetail";
import {
  useDeleteGenre,
  useGenre,
  useGenreBooks,
  useGenresList,
  useMergeGenre,
  useUpdateGenre,
} from "@/hooks/queries/genres";
import { useCan } from "@/hooks/useCan";
import { useDebounce } from "@/hooks/useDebounce";
import { useGallerySizeParam } from "@/hooks/useGallerySizeParam";
import { usePageTitle } from "@/hooks/usePageTitle";
import { writePermissionForEntity } from "@/utils/permissions";

const GenreDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const genreId = id ? parseInt(id, 10) : undefined;

  const {
    itemsPerPage,
    offset,
    settingsResolved: userSettingsResolved,
  } = useGallerySizeParam();

  const genreQuery = useGenre(genreId);
  usePageTitle(genreQuery.data?.name ?? "Genre");

  const genreBooksQuery = useGenreBooks(
    genreId,
    {
      limit: itemsPerPage,
      offset,
    },
    {
      enabled: userSettingsResolved && Boolean(genreId),
    },
  );

  const updateGenreMutation = useUpdateGenre();
  const mergeGenreMutation = useMergeGenre();
  const deleteGenreMutation = useDeleteGenre();

  const [mergeSearchRaw, setMergeSearchRaw] = useState("");
  const mergeSearch = useDebounce(mergeSearchRaw, 200, {
    immediate: (v) => v === "",
  });

  // Fires as soon as library_id is available rather than waiting for the merge
  // dialog to open. The query is cheap (50 items, single index scan) and
  // pre-fetching means the dialog opens instantly without a loading flash.
  // Only a role that can merge gets the dialog.
  const canMerge = useCan(writePermissionForEntity("genre"));
  const genresListQuery = useGenresList(
    {
      library_id: genreQuery.data?.library_id,
      limit: 50,
      search: mergeSearch || undefined,
    },
    { enabled: canMerge && !!genreQuery.data?.library_id },
  );

  const genre = genreQuery.data;
  const aliases = genre?.aliases ?? [];
  const bookCount = genre?.book_count ?? 0;

  const handleEdit = async (data: { name: string; aliases?: string[] }) => {
    if (!genreId) return;
    await updateGenreMutation.mutateAsync({
      genreId,
      payload: { name: data.name, aliases: data.aliases },
    });
  };

  const handleMerge = async (sourceId: number) => {
    if (!genreId) return;
    await mergeGenreMutation.mutateAsync({ targetId: genreId, sourceId });
  };

  const handleDelete = async () => {
    if (!genreId) return;
    await deleteGenreMutation.mutateAsync({ genreId });
    navigate(`/libraries/${libraryId}/genres`);
  };

  return (
    <ResourceDetail
      aliases={aliases}
      bookCount={bookCount}
      breadcrumbItems={[
        { label: "Genres", to: `/libraries/${libraryId}/genres` },
        { label: genre?.name ?? "" },
      ]}
      deleteConfig={{
        isPending: deleteGenreMutation.isPending,
        onDelete: handleDelete,
        disabled: bookCount > 0,
      }}
      editConfig={{
        isPending: updateGenreMutation.isPending,
        onSave: handleEdit,
      }}
      entityId={genreId!}
      entityType="genre"
      isLoading={genreQuery.isLoading}
      libraryId={libraryId!}
      mergeConfig={{
        entities:
          genresListQuery.data?.items.map((g) => ({
            id: g.id,
            name: g.name,
            count: g.book_count ?? 0,
          })) ?? [],
        isLoadingEntities: genresListQuery.isLoading,
        isPending: mergeGenreMutation.isPending,
        onMerge: handleMerge,
        onSearch: setMergeSearchRaw,
      }}
      name={genre?.name ?? ""}
      notFound={!genreQuery.isLoading && (!genreQuery.isSuccess || !genre)}
      notFoundLabel="Genre Not Found"
    >
      <BookGallerySection
        emptyMessage="This genre has no associated books."
        libraryId={libraryId!}
        query={genreBooksQuery}
        title="Books"
      />
    </ResourceDetail>
  );
};

export default GenreDetail;
