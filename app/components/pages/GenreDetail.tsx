import { useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";

import { BookGallerySection } from "@/components/library/BookGallerySection";
import { ResourceDetail } from "@/components/library/ResourceDetail";
import {
  DEFAULT_GALLERY_SIZE,
  ITEMS_PER_PAGE_BY_SIZE,
} from "@/constants/gallerySize";
import {
  useDeleteGenre,
  useGenre,
  useGenreBooks,
  useGenresList,
  useMergeGenre,
  useUpdateGenre,
} from "@/hooks/queries/genres";
import { useUserSettings } from "@/hooks/queries/settings";
import { useDebounce } from "@/hooks/useDebounce";
import { usePageTitle } from "@/hooks/usePageTitle";
import { parseGallerySize } from "@/libraries/gallerySize";
import type { GallerySize } from "@/types";

const GenreDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const genreId = id ? parseInt(id, 10) : undefined;

  const userSettingsQuery = useUserSettings();
  const userSettingsResolved =
    userSettingsQuery.isSuccess || userSettingsQuery.isError;

  const urlSize: GallerySize | null = parseGallerySize(
    searchParams.get("size"),
  );
  const savedSize: GallerySize =
    userSettingsQuery.data?.gallery_size ?? DEFAULT_GALLERY_SIZE;
  const effectiveSize: GallerySize = urlSize ?? savedSize;
  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);
  const itemsPerPage = ITEMS_PER_PAGE_BY_SIZE[effectiveSize];

  const genreQuery = useGenre(genreId);
  usePageTitle(genreQuery.data?.name ?? "Genre");

  const genreBooksQuery = useGenreBooks(
    genreId,
    {
      limit: itemsPerPage,
      offset: (currentPage - 1) * itemsPerPage,
    },
    {
      enabled: userSettingsResolved && Boolean(genreId),
    },
  );

  const updateGenreMutation = useUpdateGenre();
  const mergeGenreMutation = useMergeGenre();
  const deleteGenreMutation = useDeleteGenre();

  const [mergeSearchRaw, setMergeSearchRaw] = useState("");
  const mergeSearch = useDebounce(mergeSearchRaw, 200);

  // Fires as soon as library_id is available rather than waiting for the merge
  // dialog to open. The query is cheap (50 items, single index scan) and
  // pre-fetching means the dialog opens instantly without a loading flash.
  const genresListQuery = useGenresList(
    {
      library_id: genreQuery.data?.library_id,
      limit: 50,
      search: mergeSearch || undefined,
    },
    { enabled: !!genreQuery.data?.library_id },
  );

  const genre = genreQuery.data;
  const aliases = genre ? ((genre.aliases as unknown as string[]) ?? []) : [];
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
