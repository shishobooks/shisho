import { format, formatDistanceToNow } from "date-fns";
import { Edit, Save, Share2, Trash2 } from "lucide-react";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";

import BookItem from "@/components/library/BookItem";
import { CreateListDialog } from "@/components/library/CreateListDialog";
import { DraggableBookList } from "@/components/library/DraggableBookList";
import Gallery from "@/components/library/Gallery";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import { ShareListDialog } from "@/components/library/ShareListDialog";
import { SizeButton, SizePopover } from "@/components/library/SizePopover";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  useDeleteList,
  useList,
  useListBooks,
  useReorderListBooks,
  useUpdateList,
} from "@/hooks/queries/lists";
import { useCan } from "@/hooks/useCan";
import { useGallerySizeParam } from "@/hooks/useGallerySizeParam";
import { usePageTitle } from "@/hooks/usePageTitle";
import { isLoadFailure, toastRequestError } from "@/libraries/api";
import {
  ListSortAddedAtAsc,
  ListSortAddedAtDesc,
  ListSortAuthorAsc,
  ListSortAuthorDesc,
  ListSortTitleAsc,
  ListSortTitleDesc,
  type ListBook,
  type ListSort,
  type UpdateListPayload,
} from "@/types";

const SORT_OPTIONS = [
  { value: ListSortAddedAtDesc, label: "Recently Added" },
  { value: ListSortAddedAtAsc, label: "Oldest Added" },
  { value: ListSortTitleAsc, label: "Title (A-Z)" },
  { value: ListSortTitleDesc, label: "Title (Z-A)" },
  { value: ListSortAuthorAsc, label: "Author (A-Z)" },
  { value: ListSortAuthorDesc, label: "Author (Z-A)" },
];

const ListDetail = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const listId = id ? parseInt(id, 10) : undefined;
  // A list's books and their covers come from Books Read routes. A role
  // without it still sees the list and can manage it, but not its books.
  const canReadBooks = useCan("books:read");

  const {
    savedSize,
    effectiveSize,
    isSizeDirty,
    itemsPerPage,
    currentPage,
    offset,
    settingsResolved: userSettingsResolved,
    isSaving: isSavingSize,
    applyGallerySize,
    saveSizeAsDefault,
  } = useGallerySizeParam();

  const [sort, setSort] = useState<ListSort | undefined>(undefined);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [shareDialogOpen, setShareDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

  const listQuery = useList(listId);

  usePageTitle(listQuery.data?.name ?? "List");
  const listBooksQuery = useListBooks(
    listId,
    { sort, limit: itemsPerPage, offset },
    { enabled: userSettingsResolved && Boolean(listId) },
  );
  const updateListMutation = useUpdateList();
  const deleteListMutation = useDeleteList();
  const reorderMutation = useReorderListBooks();

  const handleReorder = async (bookIds: number[]) => {
    if (!listId) return;
    try {
      await reorderMutation.mutateAsync({
        listId,
        payload: { book_ids: bookIds },
      });
    } catch (error) {
      toastRequestError(error, "Failed to reorder list");
      throw error; // DraggableBookList restores the previous order
    }
  };

  // Permission helpers
  const permission = listQuery.data?.permission ?? "viewer";
  const isOwner = permission === "owner";
  const canManage = permission === "owner" || permission === "manager";
  const canEdit =
    permission === "owner" ||
    permission === "manager" ||
    permission === "editor";

  const handleUpdate = async (payload: UpdateListPayload) => {
    if (!listId) return;

    try {
      await updateListMutation.mutateAsync({ listId, payload });
      toast.success("List updated");
    } catch (error) {
      toastRequestError(error, "Failed to update list");
      throw error; // Let CreateListDialog preserve the draft on failure.
    }
  };

  const handleDelete = async () => {
    if (!listId) return;

    try {
      await deleteListMutation.mutateAsync({ listId });
      toast.success("List deleted");
      navigate("/lists");
    } catch (error) {
      toastRequestError(error, "Failed to delete list");
    }
  };

  if (listQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-4 md:px-6 py-4 md:py-8">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (isLoadFailure(listQuery)) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-4 md:px-6 py-4 md:py-8">
          <QueryError fallback="Failed to load list" query={listQuery} />
        </div>
      </div>
    );
  }

  if (!listQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-4 md:px-6 py-4 md:py-8">
          <div className="text-center">
            <h1 className="text-2xl font-semibold mb-4">List Not Found</h1>
            <p className="text-muted-foreground">
              The list you're looking for doesn't exist or may have been
              removed.
            </p>
          </div>
        </div>
      </div>
    );
  }

  const list = listQuery.data;
  const bookCount = listQuery.data.book_count;
  const books = listBooksQuery.data?.items ?? [];

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-4 md:px-6 py-4 md:py-8">
        {/* List Header */}
        <div className="mb-6 md:mb-8">
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-2">
            <h1 className="text-2xl font-semibold min-w-0 break-words">
              {list.name}
            </h1>
            {/* Managers and owners can also edit, so canEdit covers every action. */}
            {canEdit && (
              <div className="flex items-center gap-2 shrink-0">
                <Button
                  aria-label="Edit"
                  onClick={() => setEditDialogOpen(true)}
                  size="sm"
                  variant="outline"
                >
                  <Edit className="h-4 w-4 sm:mr-2" />
                  <span className="hidden sm:inline">Edit</span>
                </Button>
                {canManage && (
                  <Button
                    aria-label="Share"
                    onClick={() => setShareDialogOpen(true)}
                    size="sm"
                    variant="outline"
                  >
                    <Share2 className="h-4 w-4 sm:mr-2" />
                    <span className="hidden sm:inline">Share</span>
                  </Button>
                )}
                {isOwner && (
                  <Button
                    aria-label="Delete"
                    onClick={() => setDeleteDialogOpen(true)}
                    size="sm"
                    variant="outline"
                  >
                    <Trash2 className="h-4 w-4 sm:mr-2" />
                    <span className="hidden sm:inline">Delete</span>
                  </Button>
                )}
              </div>
            )}
          </div>
          {list.description && (
            <p className="text-sm md:text-base text-muted-foreground mb-2">
              {list.description}
            </p>
          )}
          <div className="flex items-center gap-2 flex-wrap">
            <Badge variant="secondary">
              {bookCount} book{bookCount !== 1 ? "s" : ""}
            </Badge>
            {!isOwner && <Badge variant="outline">{permission}</Badge>}
            {!isOwner && list.user && (
              <Badge variant="outline">Shared by {list.user.username}</Badge>
            )}
          </div>
          <p className="text-xs text-muted-foreground mt-2">
            Created {format(new Date(list.created_at), "MMM d, yyyy")} · Updated{" "}
            {formatDistanceToNow(new Date(list.updated_at), {
              addSuffix: true,
            })}
          </p>
        </div>

        {/* Sort dropdown for unordered lists */}
        {!list.is_ordered && bookCount > 0 && canReadBooks && (
          <div className="mb-6 flex items-center gap-2">
            <span
              className="text-sm text-muted-foreground"
              id="list-sort-label"
            >
              Sort by:
            </span>
            <Select
              onValueChange={(value) => setSort(value as ListSort)}
              value={sort ?? list.default_sort ?? ListSortAddedAtDesc}
            >
              <SelectTrigger aria-labelledby="list-sort-label" className="w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {SORT_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {canManage && sort && sort !== list.default_sort && (
              <Button
                disabled={updateListMutation.isPending}
                onClick={() => {
                  // handleUpdate displays the error; no dialog awaits this action.
                  void handleUpdate({ default_sort: sort }).catch(() => {});
                }}
                size="sm"
                title="Save as default sort"
                variant="ghost"
              >
                <Save className="h-4 w-4 mr-1" />
                Save as default
              </Button>
            )}
          </div>
        )}

        {bookCount > 0 && !canReadBooks && (
          <div className="text-center py-8 text-muted-foreground">
            Your role cannot view books, so the books in this list are hidden.
          </div>
        )}

        {/* Books in List */}
        {bookCount > 0 && canReadBooks && (
          <section className="mb-10">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-xl font-semibold">
                Books
                {list.is_ordered &&
                  canEdit &&
                  currentPage === 1 &&
                  bookCount <= itemsPerPage && (
                    <span className="text-sm font-normal text-muted-foreground ml-2">
                      (drag to reorder)
                    </span>
                  )}
              </h2>
              <div className="hidden sm:flex">
                <SizePopover
                  effectiveSize={effectiveSize}
                  isSaving={isSavingSize}
                  onChange={applyGallerySize}
                  onSaveAsDefault={saveSizeAsDefault}
                  savedSize={savedSize}
                  trigger={<SizeButton isDirty={isSizeDirty} />}
                />
              </div>
            </div>
            {/* Use DraggableBookList for ordered lists when on page 1 and all books fit */}
            {list.is_ordered &&
            canEdit &&
            currentPage === 1 &&
            bookCount <= itemsPerPage ? (
              listBooksQuery.error && !listBooksQuery.data ? (
                <QueryError
                  fallback="Failed to load books"
                  query={listBooksQuery}
                />
              ) : listBooksQuery.data ? (
                <DraggableBookList
                  books={books}
                  gallerySize={effectiveSize}
                  isOwner={isOwner}
                  onReorder={handleReorder}
                />
              ) : (
                // Loading, or waiting on the query's enabled gate.
                <LoadingSpinner />
              )
            ) : (
              <Gallery
                isLoading={listBooksQuery.isLoading}
                itemLabel="books"
                items={books}
                itemsPerPage={itemsPerPage}
                query={listBooksQuery}
                renderItem={(listBook: ListBook) =>
                  listBook.book ? (
                    <BookItem
                      addedByUsername={
                        !isOwner ? listBook.added_by_user?.username : undefined
                      }
                      book={listBook.book}
                      gallerySize={effectiveSize}
                      key={listBook.id}
                      libraryId={listBook.book.library_id.toString()}
                    />
                  ) : null
                }
                total={listBooksQuery.data?.total ?? bookCount}
              />
            )}
          </section>
        )}

        {/* Empty State */}
        {bookCount === 0 && (
          <div className="text-center py-8 text-muted-foreground">
            This list has no books yet.
          </div>
        )}
      </div>

      <CreateListDialog
        isPending={updateListMutation.isPending}
        list={list}
        onOpenChange={setEditDialogOpen}
        onUpdate={handleUpdate}
        open={editDialogOpen}
      />

      <ShareListDialog
        listId={listId!}
        listName={list.name}
        onOpenChange={setShareDialogOpen}
        open={shareDialogOpen}
      />

      <ConfirmDialog
        confirmLabel="Delete"
        description="Are you sure you want to delete this list? This action cannot be undone."
        isPending={deleteListMutation.isPending}
        onConfirm={handleDelete}
        onOpenChange={setDeleteDialogOpen}
        open={deleteDialogOpen}
        title="Delete List"
      />
    </div>
  );
};

export default ListDetail;
