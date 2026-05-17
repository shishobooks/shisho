import { useMemo, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";

import {
  FILE_LIST_ITEMS_PER_PAGE,
  FileListSection,
} from "@/components/library/FileListSection";
import {
  PublisherEditDialog,
  type PublisherEditData,
} from "@/components/library/PublisherEditDialog";
import { ResourceDetail } from "@/components/library/ResourceDetail";
import { useParentPublisherSearch } from "@/hooks/queries/entity-search";
import {
  useDeletePublisher,
  useMergePublisher,
  usePublisher,
  usePublisherFiles,
  usePublishersList,
  useUpdatePublisher,
} from "@/hooks/queries/publishers";
import { useDebounce } from "@/hooks/useDebounce";
import { usePageTitle } from "@/hooks/usePageTitle";

const PublisherDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const publisherId = id ? parseInt(id, 10) : undefined;

  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);

  const publisherQuery = usePublisher(publisherId);
  usePageTitle(publisherQuery.data?.name ?? "Publisher");

  const publisherFilesQuery = usePublisherFiles(
    publisherId,
    {
      limit: FILE_LIST_ITEMS_PER_PAGE,
      offset: (currentPage - 1) * FILE_LIST_ITEMS_PER_PAGE,
    },
    { enabled: Boolean(publisherId) },
  );

  const updatePublisherMutation = useUpdatePublisher();
  const mergePublisherMutation = useMergePublisher();
  const deletePublisherMutation = useDeletePublisher();

  const [mergeSearchRaw, setMergeSearchRaw] = useState("");
  const mergeSearch = useDebounce(mergeSearchRaw, 200);

  // Pre-fetch the publisher list as soon as library_id is available so the
  // merge dialog opens instantly without a loading flash.
  const publishersListQuery = usePublishersList(
    {
      library_id: publisherQuery.data?.library_id,
      limit: 50,
      search: mergeSearch || undefined,
    },
    { enabled: !!publisherQuery.data?.library_id },
  );

  const publisher = publisherQuery.data;
  const aliases = publisher
    ? ((publisher.aliases as unknown as string[]) ?? [])
    : [];
  const fileCount = publisher?.file_count ?? 0;

  // Compute exclusion list for parent publisher search (self + descendants)
  const descendantIds = publisher?.descendant_ids;
  const excludeIdsForParent = useMemo(() => {
    if (!publisherId) return [];
    const ids = [publisherId];
    if (descendantIds) {
      ids.push(...descendantIds);
    }
    return ids;
  }, [publisherId, descendantIds]);

  // Build ancestor breadcrumb items for the publisher hierarchy
  const ancestors = publisher?.ancestors;
  const ancestorBreadcrumbs = useMemo(() => {
    if (!ancestors || ancestors.length === 0) return [];
    // Ancestors come in order: immediate parent -> root
    // Reverse for breadcrumb display: root -> ... -> parent
    const reversed = [...ancestors].reverse();
    return reversed.map((ancestor) => ({
      label: ancestor.name,
      to: `/libraries/${libraryId}/publishers/${ancestor.id}`,
    }));
  }, [ancestors, libraryId]);

  // Breadcrumb chain: Publishers > [Root > ... > Parent >] Current
  const breadcrumbItems = [
    { label: "Publishers", to: `/libraries/${libraryId}/publishers` },
    ...ancestorBreadcrumbs,
    { label: publisher?.name ?? "" },
  ];

  const [editOpen, setEditOpen] = useState(false);

  // Find the parent name for the edit dialog
  const parentName = useMemo(() => {
    if (!publisher?.parent_id || !ancestors) return null;
    // The first ancestor is the immediate parent
    const immediateParent = ancestors[0];
    return immediateParent?.name ?? null;
  }, [publisher?.parent_id, ancestors]);

  const handleEdit = async (data: PublisherEditData) => {
    if (!publisherId) return;
    await updatePublisherMutation.mutateAsync({
      publisherId,
      payload: {
        name: data.name,
        aliases: data.aliases,
        parent_id: data.parent_id,
      },
    });
  };

  const handleMerge = async (sourceId: number) => {
    if (!publisherId) return;
    await mergePublisherMutation.mutateAsync({
      targetId: publisherId,
      sourceId,
    });
  };

  const handleDelete = async () => {
    if (!publisherId) return;
    await deletePublisherMutation.mutateAsync({ publisherId });
    navigate(`/libraries/${libraryId}/publishers`);
  };

  // Hook for parent publisher search, excluding self + descendants
  const useParentSearchHook = (query: string) => {
    return useParentPublisherSearch(
      publisher?.library_id,
      excludeIdsForParent,
      editOpen && !!publisher?.library_id,
      query,
    );
  };

  return (
    <>
      <ResourceDetail
        aliases={aliases}
        bookCount={fileCount}
        breadcrumbItems={breadcrumbItems}
        countLabel={{ singular: "file", plural: "files" }}
        deleteConfig={{
          isPending: deletePublisherMutation.isPending,
          onDelete: handleDelete,
          disabled: fileCount > 0,
        }}
        editConfig={{
          isPending: updatePublisherMutation.isPending,
          onSave: async (data) => {
            await handleEdit({ name: data.name, aliases: data.aliases });
          },
        }}
        entityId={publisherId!}
        entityType="publisher"
        isLoading={publisherQuery.isLoading}
        libraryId={libraryId!}
        mergeConfig={{
          entities:
            publishersListQuery.data?.items.map((p) => ({
              id: p.id,
              name: p.name,
              count: p.file_count ?? 0,
            })) ?? [],
          isLoadingEntities: publishersListQuery.isLoading,
          isPending: mergePublisherMutation.isPending,
          onMerge: handleMerge,
          onSearch: setMergeSearchRaw,
        }}
        name={publisher?.name ?? ""}
        notFound={
          !publisherQuery.isLoading && (!publisherQuery.isSuccess || !publisher)
        }
        notFoundLabel="Publisher Not Found"
        onEditClick={() => setEditOpen(true)}
      >
        <FileListSection
          emptyMessage="This publisher has no associated files."
          libraryId={libraryId!}
          query={publisherFilesQuery}
          title="Files"
        />
      </ResourceDetail>

      <PublisherEditDialog
        aliases={aliases}
        entityName={publisher?.name ?? ""}
        isPending={updatePublisherMutation.isPending}
        onOpenChange={setEditOpen}
        onSave={handleEdit}
        open={editOpen}
        parentId={publisher?.parent_id ?? null}
        parentName={parentName}
        useParentSearch={useParentSearchHook}
      />
    </>
  );
};

export default PublisherDetail;
