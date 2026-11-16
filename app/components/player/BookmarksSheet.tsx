import { Pencil, Trash2 } from "lucide-react";
import { useRef, useState } from "react";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import BookmarkNoteDialog from "@/components/player/BookmarkNoteDialog";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  useDeleteBookmark,
  useFileBookmarks,
  useUpdateBookmark,
} from "@/hooks/queries/bookmarks";
import { toastRequestError } from "@/libraries/api";
import type { Bookmark } from "@/types";
import { formatPlayerTime } from "@/utils/format";

interface BookmarksSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  fileId: number;
  /** Moves playback to a Bookmark's moment. */
  onJump: (seconds: number) => void;
}

// The user's Bookmarks in an audiobook File, in time order, each with jump,
// edit note, and delete.
const BookmarksSheet = ({
  open,
  onOpenChange,
  fileId,
  onJump,
}: BookmarksSheetProps) => {
  const query = useFileBookmarks(fileId, { enabled: open });
  const updateBookmark = useUpdateBookmark(fileId);
  const deleteBookmark = useDeleteBookmark(fileId);
  // The Bookmark being edited stays set while its dialog closes, so the
  // dialog doesn't change under its closing animation.
  const [editing, setEditing] = useState<Bookmark | null>(null);
  const [editOpen, setEditOpen] = useState(false);
  // The edit dialog has no trigger of its own, so focus goes back to the
  // Bookmark's edit button when it closes, or the panel if that's gone.
  const editButtons = useRef(new Map<number, HTMLButtonElement>());
  const panelRef = useRef<HTMLDivElement>(null);

  const bookmarks = query.data;

  const handleSaveNote = async (note: string) => {
    if (!editing) return;
    try {
      await updateBookmark.mutateAsync({
        bookmarkId: editing.id,
        payload: { note },
      });
    } catch (error) {
      toastRequestError(error, "Failed to save bookmark note");
      throw error;
    }
  };

  return (
    <>
      <Sheet onOpenChange={onOpenChange} open={open}>
        <SheetContent
          className="flex w-full flex-col sm:max-w-sm"
          ref={panelRef}
        >
          <SheetHeader>
            <SheetTitle>Bookmarks</SheetTitle>
            <SheetDescription>
              Jump back to a moment you marked.
            </SheetDescription>
          </SheetHeader>
          <div className="-mx-2 min-h-0 flex-1 overflow-y-auto px-2">
            {query.error && !bookmarks ? (
              <QueryError fallback="Failed to load bookmarks" query={query} />
            ) : !bookmarks ? (
              query.isEnabled && <LoadingSpinner />
            ) : bookmarks.length === 0 ? (
              <p className="py-4 text-sm text-muted-foreground">
                No bookmarks yet. Use Add Bookmark to mark where you are.
              </p>
            ) : (
              <ul className="space-y-2">
                {bookmarks.map((bookmark) => {
                  const time = formatPlayerTime(bookmark.seconds);
                  return (
                    <li
                      className="flex items-start gap-1 rounded-md border p-2"
                      key={bookmark.id}
                    >
                      <Button
                        className="min-w-0 flex-1 rounded-sm px-1 py-0.5 text-left hover:bg-accent"
                        onClick={() => {
                          onJump(bookmark.seconds);
                          onOpenChange(false);
                        }}
                        variant="unstyled"
                      >
                        <span className="sr-only">Jump to</span>{" "}
                        <span className="block text-sm font-medium tabular-nums">
                          {time}
                        </span>
                        {bookmark.note && (
                          <>
                            {" "}
                            <span className="block whitespace-pre-wrap break-words text-sm text-muted-foreground">
                              {bookmark.note}
                            </span>
                          </>
                        )}
                      </Button>
                      <Button
                        aria-label={`Edit note for bookmark at ${time}`}
                        onClick={() => {
                          setEditing(bookmark);
                          setEditOpen(true);
                        }}
                        ref={(button) => {
                          if (button) {
                            editButtons.current.set(bookmark.id, button);
                          } else {
                            editButtons.current.delete(bookmark.id);
                          }
                        }}
                        size="icon-sm"
                        variant="ghost"
                      >
                        <Pencil />
                      </Button>
                      <Button
                        aria-label={`Delete bookmark at ${time}`}
                        disabled={
                          deleteBookmark.isPending &&
                          deleteBookmark.variables === bookmark.id
                        }
                        onClick={() =>
                          deleteBookmark.mutate(bookmark.id, {
                            onError: (error) =>
                              toastRequestError(
                                error,
                                "Failed to delete bookmark",
                              ),
                          })
                        }
                        size="icon-sm"
                        variant="ghost"
                      >
                        <Trash2 />
                      </Button>
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        </SheetContent>
      </Sheet>
      <BookmarkNoteDialog
        initialNote={editing?.note}
        isPending={updateBookmark.isPending}
        mode="edit"
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          const button = editing && editButtons.current.get(editing.id);
          (button ?? panelRef.current)?.focus();
        }}
        onOpenChange={setEditOpen}
        onSave={handleSaveNote}
        open={editOpen}
        seconds={editing?.seconds ?? 0}
      />
    </>
  );
};

export default BookmarksSheet;
