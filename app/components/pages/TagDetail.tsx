import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { BookGallerySection } from "@/components/library/BookGallerySection";
import { ResourceDetail } from "@/components/library/ResourceDetail";
import {
  useDeleteTag,
  useMergeTag,
  useTag,
  useTagBooks,
  useTagsList,
  useUpdateTag,
} from "@/hooks/queries/tags";
import { useCan } from "@/hooks/useCan";
import { useDebounce } from "@/hooks/useDebounce";
import { useGallerySizeParam } from "@/hooks/useGallerySizeParam";
import { usePageTitle } from "@/hooks/usePageTitle";
import { writePermissionForEntity } from "@/utils/permissions";

const TagDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const tagId = id ? parseInt(id, 10) : undefined;

  const {
    itemsPerPage,
    offset,
    settingsResolved: userSettingsResolved,
  } = useGallerySizeParam();

  const tagQuery = useTag(tagId);
  usePageTitle(tagQuery.data?.name ?? "Tag");

  const tagBooksQuery = useTagBooks(
    tagId,
    {
      limit: itemsPerPage,
      offset,
    },
    {
      enabled: userSettingsResolved && Boolean(tagId),
    },
  );

  const updateTagMutation = useUpdateTag();
  const mergeTagMutation = useMergeTag();
  const deleteTagMutation = useDeleteTag();

  const [mergeSearchRaw, setMergeSearchRaw] = useState("");
  const mergeSearch = useDebounce(mergeSearchRaw, 200, {
    immediate: (v) => v === "",
  });

  // Fires as soon as library_id is available rather than waiting for the merge
  // dialog to open. The query is cheap (50 items, single index scan) and
  // pre-fetching means the dialog opens instantly without a loading flash.
  // Only a role that can merge gets the dialog.
  const canMerge = useCan(writePermissionForEntity("tag"));
  const tagsListQuery = useTagsList(
    {
      library_id: tagQuery.data?.library_id,
      limit: 50,
      search: mergeSearch || undefined,
    },
    { enabled: canMerge && !!tagQuery.data?.library_id },
  );

  const tag = tagQuery.data;
  const aliases = tag?.aliases ?? [];
  const bookCount = tag?.book_count ?? 0;

  const handleEdit = async (data: { name: string; aliases?: string[] }) => {
    if (!tagId) return;
    return updateTagMutation.mutateAsync({
      tagId,
      payload: { name: data.name, aliases: data.aliases },
    });
  };

  const handleMerge = async (sourceId: number) => {
    if (!tagId) return;
    return mergeTagMutation.mutateAsync({ targetId: tagId, sourceId });
  };

  const handleDelete = async () => {
    if (!tagId) return;
    return deleteTagMutation.mutateAsync(
      { tagId },
      { onSuccess: () => navigate(`/libraries/${libraryId}/tags`) },
    );
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
      notFoundLabel="Tag Not Found"
      query={tagQuery}
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
