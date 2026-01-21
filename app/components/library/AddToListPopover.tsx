import { List, Loader2, Plus } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  useAddBooksToList,
  useBookLists,
  useListLists,
  useRemoveBooksFromList,
  type ListWithCount,
} from "@/hooks/queries/lists";

interface AddToListPopoverProps {
  bookId: number;
  trigger?: React.ReactNode;
  onCreateList?: () => void;
}

const AddToListPopover = ({
  bookId,
  trigger,
  onCreateList,
}: AddToListPopoverProps) => {
  const [open, setOpen] = useState(false);
  const [mutatingListId, setMutatingListId] = useState<number | null>(null);

  const listsQuery = useListLists();
  const bookListsQuery = useBookLists(bookId, { enabled: open });
  const addMutation = useAddBooksToList();
  const removeMutation = useRemoveBooksFromList();

  const lists = listsQuery.data?.lists ?? [];
  const bookListIds = new Set(
    (bookListsQuery.data ?? []).map((list) => list.id),
  );

  const isLoading = listsQuery.isLoading || bookListsQuery.isLoading;
  const hasLists = lists.length > 0;

  const handleToggle = async (list: ListWithCount) => {
    const isInList = bookListIds.has(list.id);
    setMutatingListId(list.id);

    try {
      if (isInList) {
        await removeMutation.mutateAsync({
          listId: list.id,
          payload: { book_ids: [bookId] },
        });
      } else {
        await addMutation.mutateAsync({
          listId: list.id,
          payload: { book_ids: [bookId] },
        });
      }
    } finally {
      setMutatingListId(null);
    }
  };

  const handleCreateList = () => {
    setOpen(false);
    onCreateList?.();
  };

  return (
    <Popover onOpenChange={setOpen} open={open}>
      <PopoverTrigger asChild>
        {trigger ?? (
          <Button size="sm" title="Add to list" variant="ghost">
            <List className="h-4 w-4" />
          </Button>
        )}
      </PopoverTrigger>
      <PopoverContent align="end" className="w-56 p-2">
        <p className="text-xs font-medium text-muted-foreground px-2 py-1">
          Add to List
        </p>

        {isLoading && (
          <div className="flex items-center justify-center py-4">
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          </div>
        )}

        {!isLoading && !hasLists && (
          <p className="text-sm text-muted-foreground px-2 py-3 text-center">
            No lists yet
          </p>
        )}

        {!isLoading && hasLists && (
          <div className="flex flex-col gap-0.5 py-1">
            {lists.map((list) => {
              const isInList = bookListIds.has(list.id);
              const isMutating = mutatingListId === list.id;

              return (
                <button
                  className="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-accent text-left w-full text-sm disabled:opacity-50"
                  disabled={isMutating}
                  key={list.id}
                  onClick={() => handleToggle(list)}
                  type="button"
                >
                  {isMutating ? (
                    <Loader2 className="h-4 w-4 animate-spin shrink-0" />
                  ) : (
                    <Checkbox
                      checked={isInList}
                      className="pointer-events-none"
                    />
                  )}
                  <span className="truncate">{list.name}</span>
                </button>
              );
            })}
          </div>
        )}

        {!isLoading && (
          <>
            <div className="border-t my-1" />
            <Button
              className="w-full justify-start"
              onClick={handleCreateList}
              size="sm"
              variant="ghost"
            >
              <Plus className="h-4 w-4" />
              Create New List
            </Button>
          </>
        )}
      </PopoverContent>
    </Popover>
  );
};

export default AddToListPopover;
