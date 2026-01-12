import { Edit, GitMerge, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import BookItem from "@/components/library/BookItem";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { MetadataDeleteDialog } from "@/components/library/MetadataDeleteDialog";
import { MetadataEditDialog } from "@/components/library/MetadataEditDialog";
import { MetadataMergeDialog } from "@/components/library/MetadataMergeDialog";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  useDeleteTag,
  useMergeTag,
  useTag,
  useTagBooks,
  useTagsList,
  useUpdateTag,
} from "@/hooks/queries/tags";
import { useDebounce } from "@/hooks/useDebounce";

const TagDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const tagId = id ? parseInt(id, 10) : undefined;

  const tagQuery = useTag(tagId);
  const tagBooksQuery = useTagBooks(tagId);

  const [editOpen, setEditOpen] = useState(false);
  const [mergeOpen, setMergeOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [mergeSearch, setMergeSearch] = useState("");
  const debouncedMergeSearch = useDebounce(mergeSearch, 200);

  const updateTagMutation = useUpdateTag();
  const mergeTagMutation = useMergeTag();
  const deleteTagMutation = useDeleteTag();

  const tagsListQuery = useTagsList(
    {
      library_id: tagQuery.data?.library_id,
      limit: 50,
      search: debouncedMergeSearch || undefined,
    },
    { enabled: mergeOpen && !!tagQuery.data?.library_id },
  );

  const handleEdit = async (data: { name: string }) => {
    if (!tagId) return;
    await updateTagMutation.mutateAsync({
      tagId,
      payload: { name: data.name },
    });
    setEditOpen(false);
  };

  const handleMerge = async (sourceId: number) => {
    if (!tagId) return;
    await mergeTagMutation.mutateAsync({
      targetId: tagId,
      sourceId,
    });
    setMergeOpen(false);
  };

  const handleDelete = async () => {
    if (!tagId) return;
    await deleteTagMutation.mutateAsync({ tagId });
    setDeleteOpen(false);
    navigate(`/libraries/${libraryId}/tags`);
  };

  if (tagQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (!tagQuery.isSuccess || !tagQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <div className="text-center">
            <h1 className="text-2xl font-semibold mb-4">Tag Not Found</h1>
            <p className="text-muted-foreground mb-6">
              The tag you're looking for doesn't exist or may have been removed.
            </p>
            <Link
              className="text-primary hover:underline"
              to={`/libraries/${libraryId}/tags`}
            >
              Back to Tags
            </Link>
          </div>
        </div>
      </div>
    );
  }

  const tag = tagQuery.data;
  const bookCount = tag.book_count ?? 0;
  const canDelete = bookCount === 0;

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        {/* Tag Header */}
        <div className="mb-8">
          <div className="flex items-start justify-between gap-4 mb-2">
            <h1 className="text-3xl font-bold min-w-0 break-words">
              {tag.name}
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
            {bookCount} book{bookCount !== 1 ? "s" : ""}
          </Badge>
        </div>

        {/* Books with this Tag */}
        {bookCount > 0 && (
          <section className="mb-10">
            <h2 className="text-xl font-semibold mb-4">Books</h2>
            {tagBooksQuery.isLoading && <LoadingSpinner />}
            {tagBooksQuery.isSuccess && (
              <div className="flex flex-wrap gap-6">
                {tagBooksQuery.data.map((book) => (
                  <BookItem book={book} key={book.id} libraryId={libraryId!} />
                ))}
              </div>
            )}
          </section>
        )}

        {/* No Books */}
        {bookCount === 0 && (
          <div className="text-center py-8 text-muted-foreground">
            This tag has no associated books.
          </div>
        )}

        <MetadataEditDialog
          entityName={tag.name}
          entityType="tag"
          isPending={updateTagMutation.isPending}
          onOpenChange={setEditOpen}
          onSave={handleEdit}
          open={editOpen}
        />

        <MetadataMergeDialog
          entities={
            tagsListQuery.data?.tags.map((t) => ({
              id: t.id,
              name: t.name,
              count: t.book_count ?? 0,
            })) ?? []
          }
          entityType="tag"
          isLoadingEntities={tagsListQuery.isLoading}
          isPending={mergeTagMutation.isPending}
          onMerge={handleMerge}
          onOpenChange={setMergeOpen}
          onSearch={setMergeSearch}
          open={mergeOpen}
          targetId={tagId!}
          targetName={tag.name}
        />

        <MetadataDeleteDialog
          entityName={tag.name}
          entityType="tag"
          isPending={deleteTagMutation.isPending}
          onDelete={handleDelete}
          onOpenChange={setDeleteOpen}
          open={deleteOpen}
        />
      </div>
    </div>
  );
};

export default TagDetail;
