import equal from "fast-deep-equal";
import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
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
import { Label } from "@/components/ui/label";
import { UnsavedChangesDialog } from "@/components/ui/unsaved-changes-dialog";
import { useCreateJob } from "@/hooks/queries/jobs";
import {
  useReviewCriteria,
  useUpdateReviewCriteria,
} from "@/hooks/queries/review";
import { useUnsavedChanges } from "@/hooks/useUnsavedChanges";
import { JobTypeRecomputeReview } from "@/types";

import { humanizeField } from "./review-criteria-utils";

interface RecomputeDialogProps {
  open: boolean;
  overrideCount: number;
  mainFileCount: number;
  clearOverrides: boolean;
  onClearOverridesChange: (value: boolean) => void;
  onConfirm: () => void;
  onCancel: () => void;
  isPending: boolean;
}

const RecomputeDialog = ({
  open,
  overrideCount,
  mainFileCount,
  clearOverrides,
  onClearOverridesChange,
  onConfirm,
  onCancel,
  isPending,
}: RecomputeDialogProps) => {
  return (
    <Dialog
      onOpenChange={(o) => {
        if (!o) onCancel();
      }}
      open={open}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Recompute review state?</DialogTitle>
          <DialogDescription asChild>
            <div>
              <p>
                Auto-detected reviewed status will refresh based on the new
                criteria. You currently have{" "}
                <strong>
                  {overrideCount} reviewed-override
                  {overrideCount !== 1 ? "s" : ""} set out of {mainFileCount}{" "}
                  total main file{mainFileCount !== 1 ? "s" : ""}
                </strong>
                .
              </p>
            </div>
          </DialogDescription>
        </DialogHeader>
        <div className="flex items-center space-x-2 py-2">
          <Checkbox
            checked={clearOverrides}
            id="clear-overrides"
            onCheckedChange={(checked) =>
              onClearOverridesChange(checked as boolean)
            }
          />
          <Label
            className="text-sm font-normal cursor-pointer"
            htmlFor="clear-overrides"
          >
            Also clear manual overrides
          </Label>
        </div>
        <DialogFooter>
          <Button disabled={isPending} onClick={onCancel} variant="outline">
            Cancel
          </Button>
          <Button disabled={isPending} onClick={onConfirm}>
            {isPending ? "Saving..." : "Confirm"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

const ReviewCriteriaSection = () => {
  const criteriaQuery = useReviewCriteria();
  const updateMutation = useUpdateReviewCriteria();
  const createJobMutation = useCreateJob();

  const [bookFields, setBookFields] = useState<string[]>([]);
  const [audioFields, setAudioFields] = useState<string[]>([]);
  const [initialValues, setInitialValues] = useState<{
    bookFields: string[];
    audioFields: string[];
  } | null>(null);
  const [isInitialized, setIsInitialized] = useState(false);

  // Confirmation dialog state
  const [saveDialogOpen, setSaveDialogOpen] = useState(false);
  const [recomputeDialogOpen, setRecomputeDialogOpen] = useState(false);
  const [clearOverrides, setClearOverrides] = useState(false);

  // Initialize form when data loads
  useEffect(() => {
    if (criteriaQuery.isSuccess && criteriaQuery.data && !isInitialized) {
      const fields = criteriaQuery.data.book_fields;
      const audio = criteriaQuery.data.audio_fields;
      setBookFields(fields);
      setAudioFields(audio);
      setInitialValues({ bookFields: fields, audioFields: audio });
      setIsInitialized(true);
    }
  }, [criteriaQuery.isSuccess, criteriaQuery.data, isInitialized]);

  // Compute dirty state
  const hasChanges = useMemo(() => {
    if (!initialValues || !isInitialized) return false;
    return (
      !equal(bookFields, initialValues.bookFields) ||
      !equal(audioFields, initialValues.audioFields)
    );
  }, [bookFields, audioFields, initialValues, isInitialized]);

  const { showBlockerDialog, proceedNavigation, cancelNavigation } =
    useUnsavedChanges(hasChanges);

  const toggleField = (
    fields: string[],
    setFields: (v: string[]) => void,
    fieldName: string,
    checked: boolean,
  ) => {
    if (checked) {
      setFields([...fields, fieldName]);
    } else {
      setFields(fields.filter((f) => f !== fieldName));
    }
  };

  const executeSave = async (overrideClear: boolean) => {
    await updateMutation.mutateAsync({
      book_fields: bookFields,
      audio_fields: audioFields,
      clear_overrides: overrideClear,
    });
    setInitialValues({ bookFields, audioFields });
    toast.success("Review state recompute queued.");
  };

  const handleSave = async () => {
    const overrideCount = criteriaQuery.data?.override_count ?? 0;
    if (overrideCount > 0) {
      setClearOverrides(false);
      setSaveDialogOpen(true);
      return;
    }
    try {
      await executeSave(false);
    } catch {
      toast.error("Failed to save review criteria.");
    }
  };

  const handleSaveConfirm = async () => {
    setSaveDialogOpen(false);
    try {
      await executeSave(clearOverrides);
    } catch {
      toast.error("Failed to save review criteria.");
    }
  };

  const executeRecompute = async (overrideClear: boolean) => {
    await createJobMutation.mutateAsync({
      payload: {
        type: JobTypeRecomputeReview,
        data: { clear_overrides: overrideClear },
      },
    });
    toast.success("Review state recompute queued.");
  };

  const handleRecomputeNow = async () => {
    const overrideCount = criteriaQuery.data?.override_count ?? 0;
    if (overrideCount > 0) {
      setClearOverrides(false);
      setRecomputeDialogOpen(true);
      return;
    }
    try {
      await executeRecompute(false);
    } catch {
      toast.error("Failed to queue recompute job.");
    }
  };

  const handleRecomputeConfirm = async () => {
    setRecomputeDialogOpen(false);
    try {
      await executeRecompute(clearOverrides);
    } catch {
      toast.error("Failed to queue recompute job.");
    }
  };

  if (criteriaQuery.isLoading) {
    return (
      <div className="border border-border rounded-md p-4 md:p-6">
        <h2 className="text-base md:text-lg font-semibold mb-3 md:mb-4">
          Review Criteria
        </h2>
        <LoadingSpinner />
      </div>
    );
  }

  if (criteriaQuery.isError || !criteriaQuery.data) {
    return (
      <div className="border border-border rounded-md p-4 md:p-6">
        <h2 className="text-base md:text-lg font-semibold mb-3 md:mb-4">
          Review Criteria
        </h2>
        <p className="text-sm text-muted-foreground">
          Failed to load review criteria.
        </p>
      </div>
    );
  }

  const {
    universal_candidates,
    audio_candidates,
    override_count,
    main_file_count,
  } = criteriaQuery.data;

  return (
    <>
      <div className="border border-border rounded-md p-4 md:p-6">
        <h2 className="text-base md:text-lg font-semibold mb-1 md:mb-2">
          Review Criteria
        </h2>
        <p className="text-sm text-muted-foreground mb-4 md:mb-6">
          Choose which metadata fields must be present for a book to be
          considered reviewed.
        </p>

        <div className="space-y-6">
          {/* Universal fields */}
          <div className="space-y-3">
            <h3 className="text-sm font-medium">Required for all books</h3>
            <div className="space-y-2">
              {universal_candidates.map((field) => (
                <div className="flex items-center space-x-2" key={field}>
                  <Checkbox
                    checked={bookFields.includes(field)}
                    id={`book-field-${field}`}
                    onCheckedChange={(checked) =>
                      toggleField(
                        bookFields,
                        setBookFields,
                        field,
                        checked as boolean,
                      )
                    }
                  />
                  <Label
                    className="text-sm font-normal cursor-pointer"
                    htmlFor={`book-field-${field}`}
                  >
                    {humanizeField(field)}
                  </Label>
                </div>
              ))}
            </div>
          </div>

          {/* Audio-specific fields */}
          <div className="space-y-3">
            <div>
              <h3 className="text-sm font-medium">
                Required for audiobooks (additional)
              </h3>
              <p className="text-xs text-muted-foreground mt-0.5">
                These apply when a book has any audiobook file.
              </p>
            </div>
            <div className="space-y-2">
              {audio_candidates.map((field) => (
                <div className="flex items-center space-x-2" key={field}>
                  <Checkbox
                    checked={audioFields.includes(field)}
                    id={`audio-field-${field}`}
                    onCheckedChange={(checked) =>
                      toggleField(
                        audioFields,
                        setAudioFields,
                        field,
                        checked as boolean,
                      )
                    }
                  />
                  <Label
                    className="text-sm font-normal cursor-pointer"
                    htmlFor={`audio-field-${field}`}
                  >
                    {humanizeField(field)}
                  </Label>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Save button */}
        <div className="flex justify-end pt-6">
          <Button
            disabled={!hasChanges || updateMutation.isPending}
            onClick={handleSave}
          >
            {updateMutation.isPending ? "Saving..." : "Save"}
          </Button>
        </div>

        {/* Recompute now button */}
        <div className="border-t border-border pt-4 mt-4">
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
            <div>
              <p className="text-sm font-medium">Recompute review state now</p>
              <p className="text-xs text-muted-foreground">
                Re-evaluate all books against the current criteria and update
                their reviewed status.
              </p>
            </div>
            <Button
              className="shrink-0"
              disabled={createJobMutation.isPending}
              onClick={handleRecomputeNow}
              variant="outline"
            >
              {createJobMutation.isPending ? "Queuing..." : "Recompute now"}
            </Button>
          </div>
        </div>
      </div>

      {/* Save confirmation dialog (when overrides exist) */}
      <RecomputeDialog
        clearOverrides={clearOverrides}
        isPending={updateMutation.isPending}
        mainFileCount={main_file_count}
        onCancel={() => setSaveDialogOpen(false)}
        onClearOverridesChange={setClearOverrides}
        onConfirm={handleSaveConfirm}
        open={saveDialogOpen}
        overrideCount={override_count}
      />

      {/* Recompute-now confirmation dialog (when overrides exist) */}
      <RecomputeDialog
        clearOverrides={clearOverrides}
        isPending={createJobMutation.isPending}
        mainFileCount={main_file_count}
        onCancel={() => setRecomputeDialogOpen(false)}
        onClearOverridesChange={setClearOverrides}
        onConfirm={handleRecomputeConfirm}
        open={recomputeDialogOpen}
        overrideCount={override_count}
      />

      <UnsavedChangesDialog
        onDiscard={proceedNavigation}
        onStay={cancelNavigation}
        open={showBlockerDialog}
      />
    </>
  );
};

export default ReviewCriteriaSection;
