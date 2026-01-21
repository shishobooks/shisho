import { Loader2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useMoveBookToPosition } from "@/hooks/queries/lists";

interface MoveToPositionDialogProps {
  listId: number;
  bookId: number;
  bookTitle: string;
  totalBooks: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export const MoveToPositionDialog = ({
  listId,
  bookId,
  bookTitle,
  totalBooks,
  open,
  onOpenChange,
}: MoveToPositionDialogProps) => {
  const [position, setPosition] = useState<string>("1");
  const moveBookMutation = useMoveBookToPosition();

  const handleMove = async () => {
    const pos = parseInt(position, 10);
    if (isNaN(pos) || pos < 1 || pos > totalBooks) {
      toast.error(`Position must be between 1 and ${totalBooks}`);
      return;
    }

    try {
      await moveBookMutation.mutateAsync({
        listId,
        bookId,
        position: pos,
      });
      toast.success(`Moved to position ${pos}`);
      onOpenChange(false);
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to move book";
      toast.error(message);
    }
  };

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Move to Position</DialogTitle>
          <DialogDescription>
            Move "{bookTitle}" to a specific position in the list.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="position">Position (1-{totalBooks})</Label>
            <Input
              id="position"
              max={totalBooks}
              min={1}
              onChange={(e) => setPosition(e.target.value)}
              type="number"
              value={position}
            />
          </div>
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="ghost">
            Cancel
          </Button>
          <Button disabled={moveBookMutation.isPending} onClick={handleMove}>
            {moveBookMutation.isPending && (
              <Loader2 className="h-4 w-4 animate-spin" />
            )}
            Move
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
