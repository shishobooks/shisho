import { Loader2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { SortNameInput } from "@/components/common/SortNameInput";
import { Button } from "@/components/ui/button";
import {
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormDialog } from "@/components/ui/form-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
import { DataSourceManual, type DataSource } from "@/types";
import { forPerson, forTitle } from "@/utils/sortname";

export type EntityType =
  | "person"
  | "series"
  | "genre"
  | "tag"
  | "publisher"
  | "imprint";

interface MetadataEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: EntityType;
  entityName: string;
  sortName?: string;
  sortNameSource?: DataSource;
  onSave: (data: { name: string; sort_name?: string }) => Promise<void>;
  isPending: boolean;
}

const ENTITY_LABELS: Record<EntityType, string> = {
  person: "Person",
  series: "Series",
  genre: "Genre",
  tag: "Tag",
  publisher: "Publisher",
  imprint: "Imprint",
};

export function MetadataEditDialog({
  open,
  onOpenChange,
  entityType,
  entityName,
  sortName,
  sortNameSource,
  onSave,
  isPending,
}: MetadataEditDialogProps) {
  const [name, setName] = useState(entityName);
  const [editSortName, setEditSortName] = useState(sortName || "");
  const [changesSaved, setChangesSaved] = useState(false);
  // Store initial values when dialog opens - used for hasChanges comparison
  const [initialValues, setInitialValues] = useState<{
    name: string;
    sortName: string;
  } | null>(null);

  const hasSortName = entityType === "person" || entityType === "series";

  // Track previous open state to detect open transitions.
  // Start with false so that if dialog starts open, we detect it as "just opened".
  const prevOpenRef = useRef(false);

  // Initialize form only when dialog opens (closed->open transition)
  // This preserves user edits when props change while dialog is open
  useEffect(() => {
    const justOpened = open && !prevOpenRef.current;
    prevOpenRef.current = open;

    // Only initialize when dialog just opened, not on every prop change
    if (!justOpened) return;

    const initialName = entityName;
    // Semantic value for state: "" when autogenerate is ON, actual value when manual
    const semanticSortName =
      sortNameSource === DataSourceManual ? sortName || "" : "";
    // Effective value for comparison: what would be displayed (accounts for generated value)
    const generateSort =
      entityType === "person"
        ? forPerson
        : entityType === "series"
          ? forTitle
          : null;
    const effectiveSortName =
      sortName || (generateSort ? generateSort(initialName) : "");
    setName(initialName);
    setEditSortName(semanticSortName);
    setInitialValues({
      name: initialName,
      sortName: effectiveSortName,
    });
    setChangesSaved(false);
  }, [open, entityName, sortName, sortNameSource, entityType]);

  const handleSubmit = async () => {
    try {
      const data: { name: string; sort_name?: string } = { name };
      if (hasSortName) {
        // Pass empty string through - backend interprets it as "regenerate sort name"
        data.sort_name = editSortName;
      }
      await onSave(data);
      setChangesSaved(true);
      requestClose();
    } catch {
      // Error handling (e.g., toast) is done by the parent callback
    }
  };

  const hasChanges = useMemo(() => {
    if (changesSaved) return false;
    if (!initialValues) return false;
    // For sort name, compare effective values (what would be displayed), not semantic values.
    // editSortName="" means auto mode, so effective value is generated from name.
    const generateSort =
      entityType === "person"
        ? forPerson
        : entityType === "series"
          ? forTitle
          : null;
    const effectiveSortName =
      editSortName || (generateSort ? generateSort(name) : "");
    // Compare against stored initial values, not live props
    return (
      name !== initialValues.name ||
      (hasSortName && effectiveSortName !== initialValues.sortName)
    );
  }, [
    name,
    hasSortName,
    editSortName,
    changesSaved,
    initialValues,
    entityType,
  ]);

  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Edit {ENTITY_LABELS[entityType]}</DialogTitle>
          <DialogDescription>
            Update the {ENTITY_LABELS[entityType].toLowerCase()} name
            {hasSortName ? " and sort order" : ""}.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              onChange={(e) => setName(e.target.value)}
              value={name}
            />
          </div>

          {hasSortName && (
            <div className="space-y-2">
              <Label>Sort Name</Label>
              <SortNameInput
                nameValue={name}
                onChange={setEditSortName}
                sortValue={sortName || ""}
                source={sortNameSource || DataSourceManual}
                type={entityType === "person" ? "person" : "title"}
              />
            </div>
          )}
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button
            disabled={isPending || !hasChanges || !name.trim()}
            onClick={handleSubmit}
          >
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </FormDialog>
  );
}
