import { Edit, Share2, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";

import BookItem from "@/components/library/BookItem";
import { CreateListDialog } from "@/components/library/CreateListDialog";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { ShareListDialog } from "@/components/library/ShareListDialog";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
  useUpdateList,
} from "@/hooks/queries/lists";
import {
  ListSortAddedAtAsc,
  ListSortAddedAtDesc,
  ListSortAuthorAsc,
  ListSortAuthorDesc,
  ListSortTitleAsc,
  ListSortTitleDesc,
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

  const [sort, setSort] = useState<string | undefined>(undefined);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [shareDialogOpen, setShareDialogOpen] = useState(false);

  const listQuery = useList(listId);
  const listBooksQuery = useListBooks(listId, { sort });
  const updateListMutation = useUpdateList();
  const deleteListMutation = useDeleteList();

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

    const confirmed = window.confirm(
      "Are you sure you want to delete this list? This action cannot be undone.",
    );
    if (!confirmed) return;

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
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (!listQuery.isSuccess || !listQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <div className="text-center">
            <h1 className="text-2xl font-semibold mb-4">List Not Found</h1>
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
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        {/* List Header */}
        <div className="mb-8">
          <div className="flex items-start justify-between gap-4 mb-2">
            <h1 className="text-3xl font-bold min-w-0 break-words">
              {list.name}
            </h1>
            <div className="flex gap-2 shrink-0">
              {canEdit && (
                <Button
                  onClick={() => setEditDialogOpen(true)}
                  size="sm"
                  variant="outline"
                >
                  <Edit className="h-4 w-4 mr-2" />
                  Edit
                </Button>
              )}
              {canManage && (
                <Button
                  onClick={() => setShareDialogOpen(true)}
                  size="sm"
                  variant="outline"
                >
                  <Share2 className="h-4 w-4 mr-2" />
                  Share
                </Button>
              )}
              {isOwner && (
                <Button
                  disabled={deleteListMutation.isPending}
                  onClick={handleDelete}
                  size="sm"
                  variant="outline"
                >
                  <Trash2 className="h-4 w-4 mr-2" />
                  Delete
                </Button>
              )}
            </div>
          </div>
          {list.description && (
            <p className="text-muted-foreground mb-2">{list.description}</p>
          )}
          <div className="flex items-center gap-2">
            <Badge variant="secondary">
              {bookCount} book{bookCount !== 1 ? "s" : ""}
            </Badge>
            {!isOwner && <Badge variant="outline">{permission}</Badge>}
          </div>
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
          </div>
        )}

        {/* Books in List */}
        {bookCount > 0 && (
          <section className="mb-10">
            <h2 className="text-xl font-semibold mb-4">Books</h2>
            {listBooksQuery.isLoading && <LoadingSpinner />}
            {listBooksQuery.isSuccess && (
              <div className="flex flex-wrap gap-6">
                {books.map((listBook) =>
                  listBook.book ? (
                    <BookItem
                      book={listBook.book}
                      key={listBook.id}
                      libraryId={listBook.book.library_id.toString()}
                    />
                  ) : null,
                )}
              </div>
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
    </div>
  );
};

export default ListDetail;
