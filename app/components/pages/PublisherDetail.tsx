import { useMemo, useState } from "react";
import {
  Link,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";

import {
  FILE_LIST_ITEMS_PER_PAGE,
  FileListSection,
} from "@/components/library/FileListSection";
import {
  PublisherEditDialog,
  type PublisherEditData,
} from "@/components/library/PublisherEditDialog";
import { ResourceDetail } from "@/components/library/ResourceDetail";
import { Badge } from "@/components/ui/badge";
import { useParentPublisherSearch } from "@/hooks/queries/entity-search";
import {
  useDeletePublisher,
  useMergePublisher,
  usePublisher,
  usePublisherFiles,
  usePublishersList,
  useSetChildPublisher,
  useUpdatePublisher,
} from "@/hooks/queries/publishers";
import { useDebounce } from "@/hooks/useDebounce";
import { usePageTitle } from "@/hooks/usePageTitle";
import { parsePageParam } from "@/libraries/pagination";

const PublisherDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const publisherId = id ? parseInt(id, 10) : undefined;

  const currentPage = parsePageParam(searchParams.get("page"));

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
  const setChildPublisherMutation = useSetChildPublisher();
  const deletePublisherMutation = useDeletePublisher();

  const [mergeSearchRaw, setMergeSearchRaw] = useState("");
  const mergeSearch = useDebounce(mergeSearchRaw, 200, {
    immediate: (v) => v === "",
  });

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
  const aliases = publisher?.aliases ?? [];
  const fileCount = publisher?.file_count ?? 0;
  const descendantFileCount = publisher?.descendant_file_count ?? 0;
  const children = publisher?.children ?? [];

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
    const payload: PublisherEditData = {
      name: data.name,
      aliases: data.aliases,
    };
    if (data.parent_name !== undefined) {
      payload.parent_name = data.parent_name;
    } else if (data.parent_id !== undefined) {
      payload.parent_id = data.parent_id;
    }

    await updatePublisherMutation.mutateAsync({
      publisherId,
      payload,
    });
  };

  const handleMerge = async (sourceId: number) => {
    if (!publisherId) return;
    await mergePublisherMutation.mutateAsync({
      targetId: publisherId,
      sourceId,
    });
  };

  const handleSetChild = async (childId: number) => {
    if (!publisherId) return;
    await setChildPublisherMutation.mutateAsync({
      parentId: publisherId,
      childId,
    });
  };

  // IDs of ancestors of this publisher — selecting one of these as "child"
  // would create a cycle in the hierarchy
  const ancestorIds = useMemo(() => {
    if (!ancestors) return [];
    return ancestors.map((a) => a.id);
  }, [ancestors]);

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
        extraBadges={
          descendantFileCount > 0 ? (
            <Badge variant="secondary">
              {descendantFileCount}{" "}
              {descendantFileCount === 1 ? "file" : "files"} in sub-publishers
            </Badge>
          ) : undefined
        }
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
          setChildConfig: {
            onSetChild: handleSetChild,
            isPending: setChildPublisherMutation.isPending,
            disabledIds: ancestorIds,
          },
        }}
        name={publisher?.name ?? ""}
        notFound={
          !publisherQuery.isLoading && (!publisherQuery.isSuccess || !publisher)
        }
        notFoundLabel="Publisher Not Found"
        onEditClick={() => setEditOpen(true)}
      >
        {children.length > 0 && (
          <section className="mb-10">
            <h2 className="text-xl font-semibold mb-4">Child Publishers</h2>
            <div className="space-y-1">
              {children.map((child) => (
                <Link
                  className="flex items-center justify-between p-3 rounded-md hover:bg-muted/50 transition-colors"
                  key={child.id}
                  to={`/libraries/${libraryId}/publishers/${child.id}`}
                >
                  <span className="font-medium">{child.name}</span>
                  <Badge variant="secondary">
                    {child.file_count}{" "}
                    {child.file_count === 1 ? "file" : "files"}
                  </Badge>
                </Link>
              ))}
            </div>
          </section>
        )}

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
