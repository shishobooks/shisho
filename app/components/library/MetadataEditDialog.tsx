import { Loader2, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { SortNameInput } from "@/components/common/SortNameInput";
import { Badge } from "@/components/ui/badge";
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
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
import { DataSourceManual, type DataSource } from "@/types";
import { resolveAliases } from "@/utils/aliases";
import { forPerson, forTitle } from "@/utils/sortname";

export type EntityType = "person" | "series" | "genre" | "tag" | "publisher";

interface MetadataEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: EntityType;
  entityName: string;
  aliases?: string[];
  sortName?: string;
  sortNameSource?: DataSource;
  onSave: (data: {
    name: string;
    sort_name?: string;
    aliases?: string[];
  }) => Promise<void>;
  isPending: boolean;
}

const ENTITY_LABELS: Record<EntityType, string> = {
  person: "Person",
  series: "Series",
  genre: "Genre",
  tag: "Tag",
  publisher: "Publisher",
};

export function MetadataEditDialog({
  open,
  onOpenChange,
  entityType,
  entityName,
  aliases,
  sortName,
  sortNameSource,
  onSave,
  isPending,
}: MetadataEditDialogProps) {
  const [name, setName] = useState(entityName);
  const [editSortName, setEditSortName] = useState(sortName || "");
  const [editAliases, setEditAliases] = useState<string[]>([]);
  const [aliasInput, setAliasInput] = useState("");
  const [serverError, setServerError] = useState<string | null>(null);
  const [changesSaved, setChangesSaved] = useState(false);
  // Store initial values when dialog opens - used for hasChanges comparison
  const [initialValues, setInitialValues] = useState<{
    name: string;
    sortName: string;
    aliases: string[];
  } | null>(null);

  const hasSortName = entityType === "person" || entityType === "series";

  // Track the name when the dialog opened and whether it was already an alias,
  // so we can auto-add/remove it when the user renames the entity.
  const initialNameRef = useRef("");
  const initialNameWasAliasRef = useRef(false);

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
    const initialAliases = aliases ?? [];
    setName(initialName);
    setEditSortName(semanticSortName);
    setEditAliases([...initialAliases]);
    setAliasInput("");
    setServerError(null);
    setInitialValues({
      name: initialName,
      sortName: effectiveSortName,
      aliases: [...initialAliases],
    });
    setChangesSaved(false);

    // Store the initial name and whether it was already an alias
    initialNameRef.current = initialName;
    initialNameWasAliasRef.current = initialAliases.some(
      (a) => a.toLowerCase() === initialName.toLowerCase(),
    );
  }, [open, entityName, sortName, sortNameSource, entityType, aliases]);

  // Auto-add/remove the initial name as an alias when the name changes.
  // When the user renames away from the initial name, the old name auto-appears
  // as an alias so it's preserved for search. When they rename back, the
  // auto-added alias is removed (but pre-existing aliases are kept).
  const handleNameChange = useCallback((newName: string) => {
    setName(newName);

    const initName = initialNameRef.current;
    if (!initName) return;

    const nameMovedAway = newName.toLowerCase() !== initName.toLowerCase();
    const nameMovedBack = !nameMovedAway;

    setEditAliases((prev) => {
      const alreadyPresent = prev.some(
        (a) => a.toLowerCase() === initName.toLowerCase(),
      );

      if (nameMovedAway && !alreadyPresent) {
        // Auto-add the initial name as an alias
        return [...prev, initName];
      }

      if (nameMovedBack && alreadyPresent && !initialNameWasAliasRef.current) {
        // Remove the auto-added alias (but only if it wasn't pre-existing)
        return prev.filter((a) => a.toLowerCase() !== initName.toLowerCase());
      }

      return prev;
    });
  }, []);

  const handleAddAlias = () => {
    const trimmed = aliasInput.trim();
    if (!trimmed) return;
    if (editAliases.some((a) => a.toLowerCase() === trimmed.toLowerCase()))
      return;
    setEditAliases([...editAliases, trimmed]);
    setAliasInput("");
  };

  const handleRemoveAlias = (index: number) => {
    setEditAliases(editAliases.filter((_, i) => i !== index));
  };

  const handleAliasKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault();
      handleAddAlias();
    }
  };

  // Resolve pending alias input into the effective alias list for comparison
  const resolvedAliases = useMemo(
    () => resolveAliases(editAliases, aliasInput),
    [editAliases, aliasInput],
  );

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
    const aliasesChanged =
      resolvedAliases.length !== initialValues.aliases.length ||
      resolvedAliases.some((a, i) => a !== initialValues.aliases[i]);
    // Compare against stored initial values, not live props
    return (
      name !== initialValues.name ||
      (hasSortName && effectiveSortName !== initialValues.sortName) ||
      aliasesChanged
    );
  }, [
    name,
    hasSortName,
    editSortName,
    changesSaved,
    initialValues,
    entityType,
    resolvedAliases,
  ]);

  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

  const handleSubmit = async () => {
    setServerError(null);
    try {
      const data: { name: string; sort_name?: string; aliases?: string[] } = {
        name,
      };
      if (hasSortName) {
        data.sort_name = editSortName;
      }
      data.aliases = resolvedAliases;
      await onSave(data);
      setChangesSaved(true);
      requestClose();
    } catch (err) {
      if (err instanceof Error) {
        setServerError(err.message);
      }
    }
  };

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Edit {ENTITY_LABELS[entityType]}</DialogTitle>
          <DialogDescription>
            Update the {ENTITY_LABELS[entityType].toLowerCase()} name
            {hasSortName ? ", sort order," : ""} and aliases.
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-6">
          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              onChange={(e) => handleNameChange(e.target.value)}
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

          <div className="space-y-2">
            <Label htmlFor="aliases">Aliases</Label>
            <div className="flex flex-wrap gap-1.5 rounded-md border border-input bg-transparent px-3 py-1.5 focus-within:border-ring focus-within:ring-ring/50 focus-within:ring-[3px]">
              {editAliases.map((alias, index) => (
                <Badge
                  className="max-w-full gap-1 pr-1"
                  key={index}
                  variant="secondary"
                >
                  <span className="truncate" title={alias}>
                    {alias}
                  </span>
                  <button
                    aria-label={`Remove alias ${alias}`}
                    className="shrink-0 ml-0.5 rounded-sm hover:bg-muted cursor-pointer"
                    onClick={() => handleRemoveAlias(index)}
                    type="button"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </Badge>
              ))}
              <input
                className="flex-1 min-w-[120px] bg-transparent text-sm outline-none placeholder:text-muted-foreground"
                id="aliases"
                onChange={(e) => setAliasInput(e.target.value)}
                onKeyDown={handleAliasKeyDown}
                placeholder={
                  editAliases.length === 0
                    ? "Type alias and press Enter"
                    : "Add another..."
                }
                value={aliasInput}
              />
            </div>
            {serverError && (
              <p className="text-sm text-destructive">{serverError}</p>
            )}
          </div>
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
            disabled={isPending || !hasChanges || !name.trim()}
            onClick={handleSubmit}
            size="sm"
          >
            {isPending && <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />}
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </FormDialog>
  );
}
