import { Edit, GitMerge, Trash2 } from "lucide-react";
import { useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";

import BookItem from "@/components/library/BookItem";
import LibraryBreadcrumbs from "@/components/library/LibraryBreadcrumbs";
import LibraryLayout from "@/components/library/LibraryLayout";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { MetadataDeleteDialog } from "@/components/library/MetadataDeleteDialog";
import { MetadataEditDialog } from "@/components/library/MetadataEditDialog";
import { MetadataMergeDialog } from "@/components/library/MetadataMergeDialog";
import { SizeButton, SizePopover } from "@/components/library/SizePopover";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DEFAULT_GALLERY_SIZE,
  ITEMS_PER_PAGE_BY_SIZE,
} from "@/constants/gallerySize";
import { useLibrary } from "@/hooks/queries/libraries";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
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
import { pageForSizeChange, parseGallerySize } from "@/libraries/gallerySize";
import type { GallerySize } from "@/types";

const TagDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const tagId = id ? parseInt(id, 10) : undefined;

  const userSettingsQuery = useUserSettings();
  const updateUserSettings = useUpdateUserSettings();

  const urlSize: GallerySize | null = parseGallerySize(
    searchParams.get("size"),
  );
  const savedSize: GallerySize =
    userSettingsQuery.data?.gallery_size ?? DEFAULT_GALLERY_SIZE;
  const effectiveSize: GallerySize = urlSize ?? savedSize;
  const isSizeDirty = urlSize !== null && urlSize !== savedSize;

  const userSettingsResolved =
    userSettingsQuery.isSuccess || userSettingsQuery.isError;

  const offset = 0;

  const applyGallerySize = (next: GallerySize) => {
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      if (next === savedSize) {
        params.delete("size");
      } else {
        params.set("size", next);
      }
      const newPage = pageForSizeChange(offset, ITEMS_PER_PAGE_BY_SIZE[next]);
      params.set("page", String(newPage));
      return params;
    });
  };

  const handleSaveSizeAsDefault = () => {
    updateUserSettings.mutate(
      { gallery_size: effectiveSize },
      {
        onSuccess: () => {
          setSearchParams((prev) => {
            const params = new URLSearchParams(prev);
            params.delete("size");
            return params;
          });
        },
      },
    );
  };

  const libraryQuery = useLibrary(libraryId);
  const tagQuery = useTag(tagId);

  usePageTitle(tagQuery.data?.name ?? "Tag");
  const tagBooksQuery = useTagBooks(tagId, { enabled: userSettingsResolved });

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
      <LibraryLayout>
        <LoadingSpinner />
      </LibraryLayout>
    );
  }

  if (!tagQuery.isSuccess || !tagQuery.data) {
    return (
      <LibraryLayout>
        <div className="text-center">
          <h1 className="text-2xl font-semibold mb-4">Tag Not Found</h1>
          <p className="text-muted-foreground">
            The tag you're looking for doesn't exist or may have been removed.
          </p>
        </div>
      </LibraryLayout>
    );
  }

  const tag = tagQuery.data;
  const bookCount = tag.book_count ?? 0;
  const canDelete = bookCount === 0;

  return (
    <LibraryLayout>
      <LibraryBreadcrumbs
        items={[
          { label: "Tags", to: `/libraries/${libraryId}/tags` },
          { label: tag.name },
        ]}
        libraryId={libraryId!}
        libraryName={libraryQuery.data?.name}
      />

      {/* Tag Header */}
      <div className="mb-8">
        <div className="flex items-start justify-between gap-4 mb-2">
          <h1 className="text-3xl font-bold min-w-0 break-words">{tag.name}</h1>
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
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-xl font-semibold">Books</h2>
            <div className="hidden sm:flex">
              <SizePopover
                effectiveSize={effectiveSize}
                isSaving={updateUserSettings.isPending}
                onChange={applyGallerySize}
                onSaveAsDefault={handleSaveSizeAsDefault}
                savedSize={savedSize}
                trigger={<SizeButton isDirty={isSizeDirty} />}
              />
            </div>
          </div>
          {tagBooksQuery.isLoading && <LoadingSpinner />}
          {tagBooksQuery.isSuccess && (
            <div className="flex flex-wrap gap-4">
              {tagBooksQuery.data.map((book) => (
                <BookItem
                  book={book}
                  cacheKey={tagBooksQuery.dataUpdatedAt}
                  gallerySize={effectiveSize}
                  key={book.id}
                  libraryId={libraryId!}
                />
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
    </LibraryLayout>
  );
};

export default TagDetail;
