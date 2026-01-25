import { List, Loader2, X } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { useAddBooksToList, useListLists } from "@/hooks/queries/lists";
import { useBulkSelection } from "@/hooks/useBulkSelection";

export const SelectionToolbar = () => {
  const { selectedBookIds, exitSelectionMode, clearSelection } =
    useBulkSelection();
  const [popoverOpen, setPopoverOpen] = useState(false);
  const [addingToListId, setAddingToListId] = useState<number | null>(null);

  const listsQuery = useListLists();
  const addToListMutation = useAddBooksToList();

  const lists = listsQuery.data?.lists ?? [];
  const editableLists = lists.filter((list) => list.permission !== "viewer");

  const handleAddToList = async (listId: number, listName: string) => {
    setAddingToListId(listId);
    try {
      await addToListMutation.mutateAsync({
        listId,
        payload: { book_ids: selectedBookIds },
      });
      const count = selectedBookIds.length;
      toast.success(
        `Added ${count} book${count !== 1 ? "s" : ""} to "${listName}"`,
      );
      setPopoverOpen(false);
      exitSelectionMode();
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to add books to list";
      toast.error(message);
    } finally {
      setAddingToListId(null);
    }
  };

  if (selectedBookIds.length === 0) {
    return null;
  }

  return (
    <div className="fixed bottom-4 left-1/2 transform -translate-x-1/2 z-50 bg-background border rounded-lg shadow-lg px-4 py-2 flex items-center gap-4">
      <span className="text-sm font-medium">
        {selectedBookIds.length} book{selectedBookIds.length !== 1 ? "s" : ""}{" "}
        selected
      </span>

      <Popover onOpenChange={setPopoverOpen} open={popoverOpen}>
        <PopoverTrigger asChild>
          <Button size="sm" variant="default">
            <List className="h-4 w-4" />
            Add to List
          </Button>
        </PopoverTrigger>
        <PopoverContent align="center" className="w-56 p-0" side="top">
          <p className="text-xs font-medium text-muted-foreground px-3 py-2">
            Add to List
          </p>
          {listsQuery.isLoading && (
            <div className="flex items-center justify-center py-4">
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            </div>
          )}
          {!listsQuery.isLoading && editableLists.length === 0 && (
            <p className="text-sm text-muted-foreground px-3 py-3 text-center">
              No editable lists
            </p>
          )}
          {!listsQuery.isLoading && editableLists.length > 0 && (
            <div className="flex flex-col gap-0.5 px-1 pb-2">
              {editableLists.map((list) => {
                const isAdding = addingToListId === list.id;
                return (
                  <button
                    className="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-accent text-left w-full text-sm cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                    disabled={isAdding}
                    key={list.id}
                    onClick={() => handleAddToList(list.id, list.name)}
                    type="button"
                  >
                    {isAdding ? (
                      <Loader2 className="h-4 w-4 animate-spin shrink-0" />
                    ) : (
                      <List className="h-4 w-4 shrink-0 text-muted-foreground" />
                    )}
                    <span className="truncate">{list.name}</span>
                  </button>
                );
              })}
            </div>
          )}
        </PopoverContent>
      </Popover>

      <Button onClick={clearSelection} size="sm" variant="ghost">
        Clear
      </Button>

      <Button onClick={exitSelectionMode} size="sm" variant="ghost">
        <X className="h-4 w-4" />
      </Button>
    </div>
  );
};
