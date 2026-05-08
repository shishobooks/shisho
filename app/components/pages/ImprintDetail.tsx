import { useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";

import {
  FILE_LIST_ITEMS_PER_PAGE,
  FileListSection,
} from "@/components/library/FileListSection";
import { ResourceDetail } from "@/components/library/ResourceDetail";
import {
  useDeleteImprint,
  useImprint,
  useImprintFiles,
  useImprintsList,
  useMergeImprint,
  useUpdateImprint,
} from "@/hooks/queries/imprints";
import { useDebounce } from "@/hooks/useDebounce";
import { usePageTitle } from "@/hooks/usePageTitle";

const ImprintDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const imprintId = id ? parseInt(id, 10) : undefined;

  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);

  const imprintQuery = useImprint(imprintId);
  usePageTitle(imprintQuery.data?.name ?? "Imprint");

  const imprintFilesQuery = useImprintFiles(
    imprintId,
    {
      limit: FILE_LIST_ITEMS_PER_PAGE,
      offset: (currentPage - 1) * FILE_LIST_ITEMS_PER_PAGE,
    },
    { enabled: Boolean(imprintId) },
  );

  const updateImprintMutation = useUpdateImprint();
  const mergeImprintMutation = useMergeImprint();
  const deleteImprintMutation = useDeleteImprint();

  const [mergeSearchRaw, setMergeSearchRaw] = useState("");
  const mergeSearch = useDebounce(mergeSearchRaw, 200);

  // Pre-fetch the imprint list as soon as library_id is available so the
  // merge dialog opens instantly without a loading flash.
  const imprintsListQuery = useImprintsList(
    {
      library_id: imprintQuery.data?.library_id,
      limit: 50,
      search: mergeSearch || undefined,
    },
    { enabled: !!imprintQuery.data?.library_id },
  );

  const imprint = imprintQuery.data;
  const aliases = imprint
    ? ((imprint.aliases as unknown as string[]) ?? [])
    : [];
  const fileCount = imprint?.file_count ?? 0;

  const handleEdit = async (data: { name: string; aliases?: string[] }) => {
    if (!imprintId) return;
    await updateImprintMutation.mutateAsync({
      imprintId,
      payload: { name: data.name, aliases: data.aliases },
    });
  };

  const handleMerge = async (sourceId: number) => {
    if (!imprintId) return;
    await mergeImprintMutation.mutateAsync({
      targetId: imprintId,
      sourceId,
    });
  };

  const handleDelete = async () => {
    if (!imprintId) return;
    await deleteImprintMutation.mutateAsync({ imprintId });
    navigate(`/libraries/${libraryId}/imprints`);
  };

  return (
    <ResourceDetail
      aliases={aliases}
      bookCount={fileCount}
      breadcrumbItems={[
        { label: "Imprints", to: `/libraries/${libraryId}/imprints` },
        { label: imprint?.name ?? "" },
      ]}
      countLabel={{ singular: "file", plural: "files" }}
      deleteConfig={{
        isPending: deleteImprintMutation.isPending,
        onDelete: handleDelete,
        disabled: fileCount > 0,
      }}
      editConfig={{
        isPending: updateImprintMutation.isPending,
        onSave: handleEdit,
      }}
      entityId={imprintId!}
      entityType="imprint"
      isLoading={imprintQuery.isLoading}
      libraryId={libraryId!}
      mergeConfig={{
        entities:
          imprintsListQuery.data?.items.map((imp) => ({
            id: imp.id,
            name: imp.name,
            count: imp.file_count ?? 0,
          })) ?? [],
        isLoadingEntities: imprintsListQuery.isLoading,
        isPending: mergeImprintMutation.isPending,
        onMerge: handleMerge,
        onSearch: setMergeSearchRaw,
      }}
      name={imprint?.name ?? ""}
      notFound={
        !imprintQuery.isLoading && (!imprintQuery.isSuccess || !imprint)
      }
      notFoundLabel="Imprint Not Found"
    >
      <FileListSection
        emptyMessage="This imprint has no associated files."
        libraryId={libraryId!}
        query={imprintFilesQuery}
        title="Files"
      />
    </ResourceDetail>
  );
};

export default ImprintDetail;
