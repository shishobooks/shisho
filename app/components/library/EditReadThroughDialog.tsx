import { Loader2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useUpdateReadThrough } from "@/hooks/queries/reading";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
import { isDemoModeError, requestErrorMessage } from "@/libraries/api";
import type { ReadThrough } from "@/types";

interface EditReadThroughDialogProps {
  readThrough: ReadThrough;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

// Edits a Read-through's start and end dates. An empty field clears the
// date. One still underway has no end date to edit.
export const EditReadThroughDialog = ({
  readThrough,
  open,
  onOpenChange,
}: EditReadThroughDialogProps) => {
  const updateMutation = useUpdateReadThrough();
  const underway = readThrough.outcome === "underway";
  const [beginDay, setBeginDay] = useState("");
  const [endDay, setEndDay] = useState("");
  const [initialValues, setInitialValues] = useState<{
    beginDay: string;
    endDay: string;
  } | null>(null);
  const [changesSaved, setChangesSaved] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const prevOpenRef = useRef(false);
  useEffect(() => {
    const justOpened = open && !prevOpenRef.current;
    prevOpenRef.current = open;
    if (!justOpened) return;
    const initial = {
      beginDay: readThrough.begin_day ?? "",
      endDay: readThrough.end_day ?? "",
    };
    setBeginDay(initial.beginDay);
    setEndDay(initial.endDay);
    setInitialValues(initial);
    setChangesSaved(false);
    setError(null);
  }, [open, readThrough]);

  const hasChanges = useMemo(() => {
    if (changesSaved || !initialValues) return false;
    return (
      beginDay !== initialValues.beginDay || endDay !== initialValues.endDay
    );
  }, [changesSaved, initialValues, beginDay, endDay]);

  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

  const handleSave = async () => {
    setError(null);
    try {
      await updateMutation.mutateAsync({
        id: readThrough.id,
        payload: {
          begin_day: beginDay || null,
          end_day: underway ? null : endDay || null,
        },
      });
      setChangesSaved(true);
      requestClose();
    } catch (err) {
      // checkStatus has already toasted a Demo Mode rejection.
      if (isDemoModeError(err)) return;
      setError(requestErrorMessage(err, "Failed to save dates"));
    }
  };

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Edit dates</DialogTitle>
          <DialogDescription>
            Leave a date empty if you don't know it.
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="read-through-begin">Started</Label>
            <Input
              aria-describedby={error ? "read-through-error" : undefined}
              aria-invalid={!!error}
              id="read-through-begin"
              onChange={(e) => setBeginDay(e.target.value)}
              type="date"
              value={beginDay}
            />
          </div>
          {!underway && (
            <div className="space-y-2">
              <Label htmlFor="read-through-end">
                {readThrough.outcome === "abandoned" ? "Stopped" : "Finished"}
              </Label>
              <Input
                aria-describedby={error ? "read-through-error" : undefined}
                aria-invalid={!!error}
                id="read-through-end"
                onChange={(e) => setEndDay(e.target.value)}
                type="date"
                value={endDay}
              />
            </div>
          )}
          {error && (
            <p className="text-sm text-destructive" id="read-through-error">
              {error}
            </p>
          )}
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
            disabled={updateMutation.isPending || !hasChanges}
            onClick={handleSave}
            size="sm"
          >
            {updateMutation.isPending && (
              <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
            )}
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </FormDialog>
  );
};
