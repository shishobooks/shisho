import { List, Loader2, Plus, Search } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import { CreateListDialog } from "@/components/library/CreateListDialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormDialog } from "@/components/ui/form-dialog";
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
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
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
  const [changesSaved, setChangesSaved] = useState(false);
  // Store initial list IDs when dialog opens - used for hasChanges comparison
  const [initialListIds, setInitialListIds] = useState<Set<number>>(new Set());

  const listsQuery = useListLists();
  const bookListsQuery = useBookLists(bookId, { enabled: open });
  const updateBookListsMutation = useUpdateBookLists();
  const createListMutation = useCreateList();

  const lists = useMemo(
    () => listsQuery.data?.lists ?? [],
    [listsQuery.data?.lists],
  );
  const isLoading = listsQuery.isLoading || bookListsQuery.isLoading;

  // Track previous open state to detect open transitions.
  // Start with false so that if dialog starts open, we detect it as "just opened".
  const prevOpenRef = useRef(false);

  // Track previous bookId to detect when it changes while dialog is open.
  const prevBookIdRef = useRef(bookId);

  // Track whether we've initialized for this dialog session.
  // This allows data to load after open transition (async fetch).
  const initializedRef = useRef(false);

  // Initialize selected lists only when dialog opens (closed->open transition)
  // or when bookId changes, and data is available. This preserves user edits when data refetches.
  useEffect(() => {
    const justOpened = open && !prevOpenRef.current;
    const bookIdChanged = bookId !== prevBookIdRef.current;
    prevOpenRef.current = open;
    prevBookIdRef.current = bookId;

    // Reset initialization flag when dialog opens OR when bookId changes
    // (bookId change while dialog is open means we need to reinitialize)
    if (justOpened || bookIdChanged) {
      initializedRef.current = false;
      setSearch("");
      setChangesSaved(false);
    }

    // Only initialize once per dialog session, and only when data is available
    if (!open || !bookListsQuery.data || initializedRef.current) return;

    initializedRef.current = true;
    const initialIds = new Set(bookListsQuery.data.map((list) => list.id));
    setSelectedListIds(initialIds);
    setInitialListIds(initialIds);
  }, [open, bookListsQuery.data, bookId]);

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
      setChangesSaved(true);
      requestClose();
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
      // CreateListDialog handles its own closing via requestClose
      // Automatically select the newly created list
      setSelectedListIds((prev) => new Set([...prev, newList.id]));
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to create list";
      toast.error(message);
      throw error; // Re-throw so CreateListDialog knows it failed
    }
  };

  const hasChanges = useMemo(() => {
    if (changesSaved) return false;
    // Compare against stored initial state, not live query data
    if (initialListIds.size === 0 && selectedListIds.size === 0) return false;
    if (initialListIds.size !== selectedListIds.size) return true;
    for (const id of selectedListIds) {
      if (!initialListIds.has(id)) return true;
    }
    return false;
  }, [changesSaved, initialListIds, selectedListIds]);

  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

  return (
    <>
      <FormDialog
        hasChanges={hasChanges}
        onOpenChange={onOpenChange}
        open={open}
      >
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
      </FormDialog>

      <CreateListDialog
        isPending={createListMutation.isPending}
        onCreate={handleCreate}
        onOpenChange={setCreateDialogOpen}
        open={createDialogOpen}
      />
    </>
  );
};
