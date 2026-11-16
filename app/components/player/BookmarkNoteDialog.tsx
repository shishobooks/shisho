import { Loader2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormDialog } from "@/components/ui/form-dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
import { formatPlayerTime } from "@/utils/format";

// Matches the server's limit on a Bookmark's note.
const NOTE_MAX_LENGTH = 2000;

interface BookmarkNoteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** "add" creates a Bookmark, where the note is optional; "edit" changes one's note. */
  mode: "add" | "edit";
  /** The moment the Bookmark marks. */
  seconds: number;
  initialNote?: string | null;
  /**
   * Saves the note. Rejects after reporting a failure, which keeps the dialog
   * open with the draft.
   */
  onSave: (note: string) => Promise<unknown>;
  isPending: boolean;
  /**
   * Where focus goes when the dialog closes, for a dialog opened without a
   * trigger of its own. Call preventDefault() on the event to take over.
   */
  onCloseAutoFocus?: (event: Event) => void;
}

const BookmarkNoteDialog = ({
  open,
  onOpenChange,
  mode,
  seconds,
  initialNote,
  onSave,
  isPending,
  onCloseAutoFocus,
}: BookmarkNoteDialogProps) => {
  const [note, setNote] = useState("");
  const [initial, setInitial] = useState("");
  const [saved, setSaved] = useState(false);

  const prevOpenRef = useRef(false);
  useEffect(() => {
    const justOpened = open && !prevOpenRef.current;
    prevOpenRef.current = open;
    if (!justOpened) return;
    setNote(initialNote ?? "");
    setInitial(initialNote ?? "");
    setSaved(false);
  }, [open, initialNote]);

  const hasChanges = !saved && note !== initial;
  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);
  const canSave = mode === "add" || hasChanges;

  const handleSave = async () => {
    try {
      await onSave(note.trim());
      setSaved(true);
      requestClose();
    } catch {
      // The caller reported it; stay open with the draft.
    }
  };

  const time = formatPlayerTime(seconds);

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md" onCloseAutoFocus={onCloseAutoFocus}>
        <DialogHeader>
          <DialogTitle>
            {mode === "add" ? "Add Bookmark" : "Edit Bookmark Note"}
          </DialogTitle>
          <DialogDescription>
            {mode === "add"
              ? `Mark ${time} to come back to.`
              : `The bookmark at ${time}.`}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-2">
          <Label htmlFor="bookmark-note">Note</Label>
          <Textarea
            id="bookmark-note"
            maxLength={NOTE_MAX_LENGTH}
            onChange={(e) => setNote(e.target.value)}
            placeholder={mode === "add" ? "Optional" : undefined}
            rows={3}
            value={note}
          />
        </DialogBody>
        <DialogFooter>
          <Button
            onClick={() => onOpenChange(false)}
            size="sm"
            variant="outline"
          >
            Cancel
          </Button>
          <Button
            disabled={isPending || !canSave}
            onClick={handleSave}
            size="sm"
          >
            {isPending && <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />}
            {mode === "add" ? "Add Bookmark" : "Save"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </FormDialog>
  );
};

export default BookmarkNoteDialog;
