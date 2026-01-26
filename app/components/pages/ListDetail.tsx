import { format, formatDistanceToNow } from "date-fns";
import { Edit, Save, Share2, Trash2 } from "lucide-react";
import { useState } from "react";
import {
  Link,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";
import { toast } from "sonner";

import BookItem from "@/components/library/BookItem";
import { CreateListDialog } from "@/components/library/CreateListDialog";
import { DraggableBookList } from "@/components/library/DraggableBookList";
import Gallery from "@/components/library/Gallery";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { ShareListDialog } from "@/components/library/ShareListDialog";
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
import { usePageTitle } from "@/hooks/usePageTitle";
import {
  ListSortAddedAtAsc,
  ListSortAddedAtDesc,
  ListSortAuthorAsc,
  ListSortAuthorDesc,
  ListSortTitleAsc,
  ListSortTitleDesc,
  type ListBook,
  type UpdateListPayload,
} from "@/types";

const ITEMS_PER_PAGE = 24;

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
  const [searchParams] = useSearchParams();
  const listId = id ? parseInt(id, 10) : undefined;

  // Get current page from URL
  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);
  const limit = ITEMS_PER_PAGE;
  const offset = (currentPage - 1) * limit;

  const [sort, setSort] = useState<string | undefined>(undefined);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [shareDialogOpen, setShareDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

  const listQuery = useList(listId);

  usePageTitle(listQuery.data?.list?.name ?? "List");
  const listBooksQuery = useListBooks(listId, { sort, limit, offset });
  const updateListMutation = useUpdateList();
  const deleteListMutation = useDeleteList();
  const reorderMutation = useReorderListBooks();

  const handleReorder = (bookIds: number[]) => {
    if (!listId) return;
    reorderMutation.mutate({ listId, payload: { book_ids: bookIds } });
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
      setEditDialogOpen(false);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to update list",
      );
    }
  };

  const handleDelete = async () => {
    if (!listId) return;

    try {
      await deleteListMutation.mutateAsync({ listId });
      toast.success("List deleted");
      navigate("/lists");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to delete list",
      );
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

  if (!listQuery.isSuccess || !listQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-4 md:px-6 py-4 md:py-8">
          <div className="text-center">
            <h1 className="text-xl md:text-2xl font-semibold mb-4">
              List Not Found
            </h1>
            <p className="text-muted-foreground mb-6">
              The list you're looking for doesn't exist or may have been
              removed.
            </p>
            <Link className="text-primary hover:underline" to="/lists">
              Back to Lists
            </Link>
          </div>
        </div>
      </div>
    );
  }

  const list = listQuery.data.list;
  const bookCount = listQuery.data.book_count;
  const books = listBooksQuery.data?.books ?? [];

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-4 md:px-6 py-4 md:py-8">
        {/* List Header */}
        <div className="mb-6 md:mb-8">
          <div className="flex flex-col gap-3 mb-2">
            <h1 className="text-2xl md:text-3xl font-bold min-w-0 break-words">
              {list.name}
            </h1>
            <div className="flex gap-2">
              {canEdit && (
                <Button
                  onClick={() => setEditDialogOpen(true)}
                  size="sm"
                  variant="outline"
                >
                  <Edit className="h-4 w-4 sm:mr-2" />
                  <span className="hidden sm:inline">Edit</span>
                </Button>
              )}
              {canManage && (
                <Button
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
                  onClick={() => setDeleteDialogOpen(true)}
                  size="sm"
                  variant="outline"
                >
                  <Trash2 className="h-4 w-4 sm:mr-2" />
                  <span className="hidden sm:inline">Delete</span>
                </Button>
              )}
            </div>
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
        {!list.is_ordered && bookCount > 0 && (
          <div className="mb-6 flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Sort by:</span>
            <Select
              onValueChange={setSort}
              value={sort ?? list.default_sort ?? ListSortAddedAtDesc}
            >
              <SelectTrigger className="w-48">
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
                onClick={() => handleUpdate({ default_sort: sort })}
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

        {/* Books in List */}
        {bookCount > 0 && (
          <section className="mb-10">
            <h2 className="text-xl font-semibold mb-4">
              Books
              {list.is_ordered &&
                canEdit &&
                currentPage === 1 &&
                bookCount <= ITEMS_PER_PAGE && (
                  <span className="text-sm font-normal text-muted-foreground ml-2">
                    (drag to reorder)
                  </span>
                )}
            </h2>
            {/* Use DraggableBookList for ordered lists when on page 1 and all books fit */}
            {list.is_ordered &&
            canEdit &&
            currentPage === 1 &&
            bookCount <= ITEMS_PER_PAGE ? (
              listBooksQuery.isLoading ? (
                <LoadingSpinner />
              ) : listBooksQuery.isSuccess ? (
                <DraggableBookList
                  books={books}
                  isOwner={isOwner}
                  onReorder={handleReorder}
                />
              ) : (
                <div>Error loading books</div>
              )
            ) : (
              <Gallery
                isLoading={listBooksQuery.isLoading}
                isSuccess={listBooksQuery.isSuccess}
                itemLabel="books"
                items={books}
                itemsPerPage={ITEMS_PER_PAGE}
                renderItem={(listBook: ListBook) =>
                  listBook.book ? (
                    <BookItem
                      addedByUsername={
                        !isOwner ? listBook.added_by_user?.username : undefined
                      }
                      book={listBook.book}
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
