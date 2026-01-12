import { Edit, GitMerge, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { MetadataDeleteDialog } from "@/components/library/MetadataDeleteDialog";
import { MetadataEditDialog } from "@/components/library/MetadataEditDialog";
import { MetadataMergeDialog } from "@/components/library/MetadataMergeDialog";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  useDeletePublisher,
  useMergePublisher,
  usePublisher,
  usePublisherFiles,
  usePublishersList,
  useUpdatePublisher,
} from "@/hooks/queries/publishers";
import { useDebounce } from "@/hooks/useDebounce";
import type { File } from "@/types";

const PublisherDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const publisherId = id ? parseInt(id, 10) : undefined;

  const publisherQuery = usePublisher(publisherId);
  const publisherFilesQuery = usePublisherFiles(publisherId);

  const [editOpen, setEditOpen] = useState(false);
  const [mergeOpen, setMergeOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [mergeSearch, setMergeSearch] = useState("");
  const debouncedMergeSearch = useDebounce(mergeSearch, 200);

  const updatePublisherMutation = useUpdatePublisher();
  const mergePublisherMutation = useMergePublisher();
  const deletePublisherMutation = useDeletePublisher();

  const publishersListQuery = usePublishersList(
    {
      library_id: publisherQuery.data?.library_id,
      limit: 50,
      search: debouncedMergeSearch || undefined,
    },
    { enabled: mergeOpen && !!publisherQuery.data?.library_id },
  );

  const handleEdit = async (data: { name: string }) => {
    if (!publisherId) return;
    await updatePublisherMutation.mutateAsync({
      publisherId,
      payload: { name: data.name },
    });
    setEditOpen(false);
  };

  const handleMerge = async (sourceId: number) => {
    if (!publisherId) return;
    await mergePublisherMutation.mutateAsync({
      targetId: publisherId,
      sourceId,
    });
    setMergeOpen(false);
  };

  const handleDelete = async () => {
    if (!publisherId) return;
    await deletePublisherMutation.mutateAsync({ publisherId });
    setDeleteOpen(false);
    navigate(`/libraries/${libraryId}/publishers`);
  };

  if (publisherQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (!publisherQuery.isSuccess || !publisherQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <div className="text-center">
            <h1 className="text-2xl font-semibold mb-4">Publisher Not Found</h1>
            <p className="text-muted-foreground mb-6">
              The publisher you're looking for doesn't exist or may have been
              removed.
            </p>
            <Link
              className="text-primary hover:underline"
              to={`/libraries/${libraryId}/publishers`}
            >
              Back to Publishers
            </Link>
          </div>
        </div>
      </div>
    );
  }

  const publisher = publisherQuery.data;
  const fileCount = publisher.file_count ?? 0;
  const canDelete = fileCount === 0;

  const getFileName = (file: File) => {
    const parts = file.filepath.split("/");
    return parts[parts.length - 1];
  };

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        {/* Publisher Header */}
        <div className="mb-8">
          <div className="flex items-start justify-between gap-4 mb-2">
            <h1 className="text-3xl font-bold min-w-0 break-words">
              {publisher.name}
            </h1>
            <div className="flex gap-2 shrink-0">
              <Button
                onClick={() => setEditOpen(true)}
                size="sm"
                variant="outline"
              >
                <Edit className="h-4 w-4 mr-2" />
                Edit
              </Button>
              <Button
                onClick={() => setMergeOpen(true)}
                size="sm"
                variant="outline"
              >
                <GitMerge className="h-4 w-4 mr-2" />
                Merge
              </Button>
              {canDelete && (
                <Button
                  onClick={() => setDeleteOpen(true)}
                  size="sm"
                  variant="outline"
                >
                  <Trash2 className="h-4 w-4 mr-2" />
                  Delete
                </Button>
              )}
            </div>
          </div>
          <Badge variant="secondary">
            {fileCount} file{fileCount !== 1 ? "s" : ""}
          </Badge>
        </div>

        {/* Files with this Publisher */}
        {fileCount > 0 && (
          <section className="mb-10">
            <h2 className="text-xl font-semibold mb-4">Files</h2>
            {publisherFilesQuery.isLoading && <LoadingSpinner />}
            {publisherFilesQuery.isSuccess && (
              <div className="space-y-3">
                {publisherFilesQuery.data.map((file) => (
                  <div
                    className="border-l-4 border-l-primary dark:border-l-violet-300 pl-4 py-2"
                    key={file.id}
                  >
                    <div className="flex items-center justify-between gap-4">
                      <div className="min-w-0 flex-1">
                        <Link
                          className="font-medium hover:underline block truncate"
                          to={`/libraries/${libraryId}/books/${file.book_id}`}
                        >
                          {file.book?.title ?? "Unknown Book"}
                        </Link>
                        <p className="text-sm text-muted-foreground truncate">
                          {getFileName(file)}
                        </p>
                      </div>
                      <Badge variant="outline">
                        {file.file_type?.toUpperCase()}
                      </Badge>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </section>
        )}

        {/* No Files */}
        {fileCount === 0 && (
          <div className="text-center py-8 text-muted-foreground">
            This publisher has no associated files.
          </div>
        )}

        <MetadataEditDialog
          entityName={publisher.name}
          entityType="publisher"
          isPending={updatePublisherMutation.isPending}
          onOpenChange={setEditOpen}
          onSave={handleEdit}
          open={editOpen}
        />

        <MetadataMergeDialog
          entities={
            publishersListQuery.data?.publishers.map((p) => ({
              id: p.id,
              name: p.name,
              count: p.file_count ?? 0,
            })) ?? []
          }
          entityType="publisher"
          isLoadingEntities={publishersListQuery.isLoading}
          isPending={mergePublisherMutation.isPending}
          onMerge={handleMerge}
          onOpenChange={setMergeOpen}
          onSearch={setMergeSearch}
          open={mergeOpen}
          targetId={publisherId!}
          targetName={publisher.name}
        />

        <MetadataDeleteDialog
          entityName={publisher.name}
          entityType="publisher"
          isPending={deletePublisherMutation.isPending}
          onDelete={handleDelete}
          onOpenChange={setDeleteOpen}
          open={deleteOpen}
        />
      </div>
    </div>
  );
};

export default PublisherDetail;
