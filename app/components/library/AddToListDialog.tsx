import { List, Loader2, Plus, Search } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { CreateListDialog } from "@/components/library/CreateListDialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  useBookLists,
  useCreateList,
  useListLists,
  useUpdateBookLists,
  type ListWithCount,
} from "@/hooks/queries/lists";
import type { CreateListPayload } from "@/types";

interface AddToListDialogProps {
  bookId: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export const AddToListDialog = ({
  bookId,
  open,
  onOpenChange,
}: AddToListDialogProps) => {
  const [search, setSearch] = useState("");
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [selectedListIds, setSelectedListIds] = useState<Set<number>>(
    new Set(),
  );

  const listsQuery = useListLists();
  const bookListsQuery = useBookLists(bookId, { enabled: open });
  const updateBookListsMutation = useUpdateBookLists();
  const createListMutation = useCreateList();

  const lists = useMemo(
    () => listsQuery.data?.lists ?? [],
    [listsQuery.data?.lists],
  );
  const isLoading = listsQuery.isLoading || bookListsQuery.isLoading;

  // Initialize selected lists when data loads
  useEffect(() => {
    if (bookListsQuery.data) {
      setSelectedListIds(new Set(bookListsQuery.data.map((list) => list.id)));
    }
  }, [bookListsQuery.data]);

  // Reset search when dialog opens
  useEffect(() => {
    if (open) {
      setSearch("");
    }
  }, [open]);

  // Filter lists by search
  const filteredLists = useMemo(() => {
    if (!search) return lists;
    const searchLower = search.toLowerCase();
    return lists.filter((list) =>
      list.name.toLowerCase().includes(searchLower),
    );
  }, [lists, search]);

  const handleToggle = (list: ListWithCount) => {
    // Viewer-only lists can't be toggled
    if (list.permission === "viewer") return;

    setSelectedListIds((prev) => {
      const next = new Set(prev);
      if (next.has(list.id)) {
        next.delete(list.id);
      } else {
        next.add(list.id);
      }
      return next;
    });
  };

  const handleSave = async () => {
    try {
      await updateBookListsMutation.mutateAsync({
        bookId,
        listIds: Array.from(selectedListIds),
      });
      toast.success("Updated lists");
      onOpenChange(false);
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to update lists";
      toast.error(message);
    }
  };

  const handleCreateList = () => {
    setCreateDialogOpen(true);
  };

  const handleCreate = async (payload: CreateListPayload) => {
    try {
      const newList = await createListMutation.mutateAsync(payload);
      toast.success(`Created "${payload.name}" list`);
      setCreateDialogOpen(false);
      // Automatically select the newly created list
      setSelectedListIds((prev) => new Set([...prev, newList.id]));
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to create list";
      toast.error(message);
    }
  };

  const hasChanges = useMemo(() => {
    if (!bookListsQuery.data) return false;
    const originalIds = new Set(bookListsQuery.data.map((list) => list.id));
    if (originalIds.size !== selectedListIds.size) return true;
    for (const id of selectedListIds) {
      if (!originalIds.has(id)) return true;
    }
    return false;
  }, [bookListsQuery.data, selectedListIds]);

  return (
    <>
      <Dialog onOpenChange={onOpenChange} open={open}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <List className="h-5 w-5" />
              Manage Lists
            </DialogTitle>
            <DialogDescription>
              Select which lists this book should belong to.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            {/* Search */}
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                className="pl-9"
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Search lists..."
                value={search}
              />
            </div>

            {/* Lists */}
            {isLoading && (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              </div>
            )}

            {!isLoading && filteredLists.length === 0 && (
              <div className="text-sm text-muted-foreground text-center py-8">
                {search ? "No lists match your search" : "No lists yet"}
              </div>
            )}

            {!isLoading && filteredLists.length > 0 && (
              <ScrollArea className="h-[240px] border rounded-md">
                <div className="p-2 space-y-1">
                  {filteredLists.map((list) => {
                    const isSelected = selectedListIds.has(list.id);
                    const isViewerOnly = list.permission === "viewer";

                    const listItem = (
                      <div
                        aria-checked={isSelected}
                        aria-disabled={isViewerOnly}
                        className="flex items-center gap-3 px-2 py-2 rounded-md hover:bg-accent text-left w-full cursor-pointer aria-disabled:opacity-50 aria-disabled:cursor-not-allowed"
                        key={list.id}
                        onClick={() => !isViewerOnly && handleToggle(list)}
                        onKeyDown={(e) => {
                          if (
                            !isViewerOnly &&
                            (e.key === "Enter" || e.key === " ")
                          ) {
                            e.preventDefault();
                            handleToggle(list);
                          }
                        }}
                        role="menuitemcheckbox"
                        tabIndex={isViewerOnly ? -1 : 0}
                      >
                        <Checkbox
                          checked={isSelected}
                          className="pointer-events-none"
                          disabled={isViewerOnly}
                          tabIndex={-1}
                        />
                        <div className="flex-1 min-w-0">
                          <div className="font-medium truncate">
                            {list.name}
                          </div>
                          <div className="text-xs text-muted-foreground">
                            {list.book_count} book
                            {list.book_count !== 1 ? "s" : ""} ·{" "}
                            {list.permission}
                          </div>
                        </div>
                      </div>
                    );

                    if (isViewerOnly) {
                      return (
                        <Tooltip key={list.id}>
                          <TooltipTrigger asChild>{listItem}</TooltipTrigger>
                          <TooltipContent>
                            You can only view this list
                          </TooltipContent>
                        </Tooltip>
                      );
                    }

                    return listItem;
                  })}
                </div>
              </ScrollArea>
            )}

            {/* Create New List */}
            {!isLoading && (
              <Button
                className="w-full"
                onClick={handleCreateList}
                variant="outline"
              >
                <Plus className="h-4 w-4" />
                Create New List
              </Button>
            )}
          </div>

          <DialogFooter>
            <Button onClick={() => onOpenChange(false)} variant="ghost">
              Cancel
            </Button>
            <Button
              disabled={!hasChanges || updateBookListsMutation.isPending}
              onClick={handleSave}
            >
              {updateBookListsMutation.isPending && (
                <Loader2 className="h-4 w-4 animate-spin" />
              )}
              Save Changes
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <CreateListDialog
        isPending={createListMutation.isPending}
        onCreate={handleCreate}
        onOpenChange={setCreateDialogOpen}
        open={createDialogOpen}
      />
    </>
  );
};
