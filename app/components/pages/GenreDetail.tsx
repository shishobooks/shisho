import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { BookGallerySection } from "@/components/library/BookGallerySection";
import { ResourceDetail } from "@/components/library/ResourceDetail";
import { useBooks } from "@/hooks/queries/books";
import {
  useDeleteGenre,
  useGenre,
  useGenresList,
  useMergeGenre,
  useUpdateGenre,
} from "@/hooks/queries/genres";
import { useCan } from "@/hooks/useCan";
import { useDebounce } from "@/hooks/useDebounce";
import { useGallerySizeParam } from "@/hooks/useGallerySizeParam";
import { usePageTitle } from "@/hooks/usePageTitle";
import { useSortParam } from "@/hooks/useSortParam";
import { BOOK_DETAIL_SORT } from "@/libraries/sortSpec";
import { formatCount } from "@/utils/format";
import { writePermissionForEntity } from "@/utils/permissions";

const GenreDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const genreId = id ? parseInt(id, 10) : undefined;

  const { itemsPerPage, offset, pageSizeResolved } = useGallerySizeParam();

  const genreQuery = useGenre(genreId);
  usePageTitle(genreQuery.data?.name ?? "Genre");

  const sort = useSortParam(BOOK_DETAIL_SORT);
  const genreBooksQuery = useBooks(
    {
      genre_ids: genreId ? [genreId] : undefined,
      library_id: libraryId ? parseInt(libraryId, 10) : undefined,
      limit: itemsPerPage,
      offset,
      sort: sort.param,
    },
    {
      enabled: pageSizeResolved && Boolean(genreId),
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
    return updateGenreMutation.mutateAsync({
      genreId,
      payload: { name: data.name, aliases: data.aliases },
    });
  };

  const handleMerge = async (sourceId: number) => {
    if (!genreId) return;
    return mergeGenreMutation.mutateAsync({ targetId: genreId, sourceId });
  };

  const handleDelete = async () => {
    if (!genreId) return;
    return deleteGenreMutation.mutateAsync(
      { genreId },
      { onSuccess: () => navigate(`/libraries/${libraryId}/genres`) },
    );
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
      libraryId={libraryId!}
      mergeConfig={{
        entities:
          genresListQuery.data?.items.map((g) => ({
            id: g.id,
            name: g.name,
            countText: formatCount(g.book_count ?? 0, "book"),
          })) ?? [],
        isLoadingEntities: genresListQuery.isLoading,
        isSearchPending:
          mergeSearchRaw !== mergeSearch || genresListQuery.isPlaceholderData,
        isPending: mergeGenreMutation.isPending,
        onMerge: handleMerge,
        onSearch: setMergeSearchRaw,
      }}
      name={genre?.name ?? ""}
      notFoundLabel="Genre Not Found"
      query={genreQuery}
    >
      <BookGallerySection
        emptyMessage="This genre has no associated books."
        libraryId={libraryId!}
        query={genreBooksQuery}
        sort={sort}
        title="Books"
      />
    </ResourceDetail>
  );
};

export default GenreDetail;
