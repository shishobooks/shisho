import { useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";

import { BookGallerySection } from "@/components/library/BookGallerySection";
import { ResourceDetail } from "@/components/library/ResourceDetail";
import {
  DEFAULT_GALLERY_SIZE,
  ITEMS_PER_PAGE_BY_SIZE,
} from "@/constants/gallerySize";
import { useUserSettings } from "@/hooks/queries/settings";
import {
  useDeleteTag,
  useMergeTag,
  useTag,
  useTagBooks,
  useTagsList,
  useUpdateTag,
} from "@/hooks/queries/tags";
import { useDebounce } from "@/hooks/useDebounce";
import { usePageTitle } from "@/hooks/usePageTitle";
import { parseGallerySize } from "@/libraries/gallerySize";
import type { GallerySize } from "@/types";

const TagDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const tagId = id ? parseInt(id, 10) : undefined;

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

  const tagQuery = useTag(tagId);
  usePageTitle(tagQuery.data?.name ?? "Tag");

  const tagBooksQuery = useTagBooks(
    tagId,
    {
      limit: itemsPerPage,
      offset: (currentPage - 1) * itemsPerPage,
    },
    {
      enabled: userSettingsResolved && Boolean(tagId),
    },
  );

  const updateTagMutation = useUpdateTag();
  const mergeTagMutation = useMergeTag();
  const deleteTagMutation = useDeleteTag();

  const [mergeSearchRaw, setMergeSearchRaw] = useState("");
  const mergeSearch = useDebounce(mergeSearchRaw, 200);

  // Fires as soon as library_id is available rather than waiting for the merge
  // dialog to open. The query is cheap (50 items, single index scan) and
  // pre-fetching means the dialog opens instantly without a loading flash.
  const tagsListQuery = useTagsList(
    {
      library_id: tagQuery.data?.library_id,
      limit: 50,
      search: mergeSearch || undefined,
    },
    { enabled: !!tagQuery.data?.library_id },
  );

  const tag = tagQuery.data;
  const aliases = tag ? ((tag.aliases as unknown as string[]) ?? []) : [];
  const bookCount = tag?.book_count ?? 0;

  const handleEdit = async (data: { name: string; aliases?: string[] }) => {
    if (!tagId) return;
    await updateTagMutation.mutateAsync({
      tagId,
      payload: { name: data.name, aliases: data.aliases },
    });
  };

  const handleMerge = async (sourceId: number) => {
    if (!tagId) return;
    await mergeTagMutation.mutateAsync({ targetId: tagId, sourceId });
  };

  const handleDelete = async () => {
    if (!tagId) return;
    await deleteTagMutation.mutateAsync({ tagId });
    navigate(`/libraries/${libraryId}/tags`);
  };

  return (
    <ResourceDetail
      aliases={aliases}
      bookCount={bookCount}
      breadcrumbItems={[
        { label: "Tags", to: `/libraries/${libraryId}/tags` },
        { label: tag?.name ?? "" },
      ]}
      deleteConfig={{
        isPending: deleteTagMutation.isPending,
        onDelete: handleDelete,
        disabled: bookCount > 0,
      }}
      editConfig={{
        isPending: updateTagMutation.isPending,
        onSave: handleEdit,
      }}
      entityId={tagId!}
      entityType="tag"
      isLoading={tagQuery.isLoading}
      libraryId={libraryId!}
      mergeConfig={{
        entities:
          tagsListQuery.data?.items.map((t) => ({
            id: t.id,
            name: t.name,
            count: t.book_count ?? 0,
          })) ?? [],
        isLoadingEntities: tagsListQuery.isLoading,
        isPending: mergeTagMutation.isPending,
        onMerge: handleMerge,
        onSearch: setMergeSearchRaw,
      }}
      name={tag?.name ?? ""}
      notFound={!tagQuery.isLoading && (!tagQuery.isSuccess || !tag)}
      notFoundLabel="Tag Not Found"
    >
      <BookGallerySection
        emptyMessage="This tag has no associated books."
        libraryId={libraryId!}
        query={tagBooksQuery}
        title="Books"
      />
    </ResourceDetail>
  );
};

export default TagDetail;
