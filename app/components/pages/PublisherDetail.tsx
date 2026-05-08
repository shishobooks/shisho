import { useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";

import { FileListSection } from "@/components/library/FileListSection";
import { ResourceDetail } from "@/components/library/ResourceDetail";
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

const ITEMS_PER_PAGE = 50;

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
      limit: ITEMS_PER_PAGE,
      offset: (currentPage - 1) * ITEMS_PER_PAGE,
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

  const handleEdit = async (data: { name: string; aliases?: string[] }) => {
    if (!publisherId) return;
    await updatePublisherMutation.mutateAsync({
      publisherId,
      payload: { name: data.name, aliases: data.aliases },
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

  return (
    <ResourceDetail
      aliases={aliases}
      bookCount={fileCount}
      breadcrumbItems={[
        { label: "Publishers", to: `/libraries/${libraryId}/publishers` },
        { label: publisher?.name ?? "" },
      ]}
      countLabel={{ singular: "file", plural: "files" }}
      deleteConfig={{
        isPending: deletePublisherMutation.isPending,
        onDelete: handleDelete,
        disabled: fileCount > 0,
      }}
      editConfig={{
        isPending: updatePublisherMutation.isPending,
        onSave: handleEdit,
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
    >
      <FileListSection
        emptyMessage="This publisher has no associated files."
        libraryId={libraryId!}
        query={publisherFilesQuery}
        title="Files"
      />
    </ResourceDetail>
  );
};

export default PublisherDetail;
