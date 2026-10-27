import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { BookGallerySection } from "@/components/library/BookGallerySection";
import { ResourceDetail } from "@/components/library/ResourceDetail";
import { useBooks } from "@/hooks/queries/books";
import { useUserLibrary } from "@/hooks/queries/libraries";
import {
  useDeleteTag,
  useMergeTag,
  useTag,
  useTagsList,
  useUpdateTag,
} from "@/hooks/queries/tags";
import { useCan } from "@/hooks/useCan";
import { useDebounce } from "@/hooks/useDebounce";
import { useGallerySizeParam } from "@/hooks/useGallerySizeParam";
import { usePageTitle } from "@/hooks/usePageTitle";
import { useSortParam } from "@/hooks/useSortParam";
import { BOOK_DETAIL_SORT } from "@/libraries/sortSpec";
import { formatCount } from "@/utils/format";
import { writePermissionForEntity } from "@/utils/permissions";
import { splitTagNamespace } from "@/utils/tagNamespaces";

const TagDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const tagId = id ? parseInt(id, 10) : undefined;

  const { itemsPerPage, offset, pageSizeResolved } = useGallerySizeParam();

  const tagQuery = useTag(tagId);
  usePageTitle(tagQuery.data?.name ?? "Tag");

  const sort = useSortParam(BOOK_DETAIL_SORT);
  const tagBooksQuery = useBooks(
    {
      tag_ids: tagId ? [tagId] : undefined,
      library_id: libraryId ? parseInt(libraryId, 10) : undefined,
      limit: itemsPerPage,
      offset,
      sort: sort.param,
    },
    {
      enabled: pageSizeResolved && Boolean(tagId),
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
  const library = useUserLibrary(libraryId);
  const split = splitTagNamespace(
    tag?.name ?? "",
    library.data?.tag_namespaces ?? [],
  );
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
        ...(split.namespace
          ? [
              {
                label: split.namespace,
                to: `/libraries/${libraryId}/tags?namespace=${encodeURIComponent(split.namespace)}`,
              },
            ]
          : []),
        { label: split.name },
      ]}
      deleteConfig={{
        isPending: deleteTagMutation.isPending,
        onDelete: handleDelete,
        disabled: bookCount > 0,
      }}
      displayName={split.name}
      editConfig={{
        isPending: updateTagMutation.isPending,
        onSave: handleEdit,
      }}
      entityId={tagId!}
      entityType="tag"
      eyebrow={split.namespace ?? undefined}
      libraryId={libraryId!}
      mergeConfig={{
        entities:
          tagsListQuery.data?.items.map((t) => ({
            id: t.id,
            name: t.name,
            countText: formatCount(t.book_count ?? 0, "book"),
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
        sort={sort}
        title="Books"
      />
    </ResourceDetail>
  );
};

export default TagDetail;
