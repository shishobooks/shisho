import { Loader2, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import {
  EntityCombobox,
  type EntityComboboxProps,
} from "@/components/common/EntityCombobox";
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
import type { PublisherIdOption } from "@/hooks/queries/entity-search";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
import { resolveAliases } from "@/utils/aliases";

export interface PublisherEditData {
  name: string;
  aliases?: string[];
  parent_id?: number | null;
}

interface PublisherEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityName: string;
  aliases?: string[];
  parentId?: number | null;
  parentName?: string | null;
  onSave: (data: PublisherEditData) => Promise<void>;
  isPending: boolean;
  useParentSearch: EntityComboboxProps<PublisherIdOption>["hook"];
}

export function PublisherEditDialog({
  open,
  onOpenChange,
  entityName,
  aliases,
  parentId,
  parentName,
  onSave,
  isPending,
  useParentSearch,
}: PublisherEditDialogProps) {
  const [name, setName] = useState(entityName);
  const [editAliases, setEditAliases] = useState<string[]>([]);
  const [aliasInput, setAliasInput] = useState("");
  const [selectedParent, setSelectedParent] =
    useState<PublisherIdOption | null>(null);
  const [parentCleared, setParentCleared] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);
  const [changesSaved, setChangesSaved] = useState(false);
  const [initialValues, setInitialValues] = useState<{
    name: string;
    aliases: string[];
    parentId: number | null;
  } | null>(null);

  // Track the name when the dialog opened and whether it was already an alias,
  // so we can auto-add/remove it when the user renames the entity.
  const initialNameRef = useRef("");
  const initialNameWasAliasRef = useRef(false);

  const prevOpenRef = useRef(false);

  useEffect(() => {
    const justOpened = open && !prevOpenRef.current;
    prevOpenRef.current = open;

    if (!justOpened) return;

    const initialName = entityName;
    const initialAliases = aliases ?? [];
    setName(initialName);
    setEditAliases([...initialAliases]);
    setAliasInput("");
    setServerError(null);
    setChangesSaved(false);
    setParentCleared(false);

    if (parentId && parentName) {
      setSelectedParent({ id: parentId, name: parentName, file_count: 0 });
    } else {
      setSelectedParent(null);
    }

    setInitialValues({
      name: initialName,
      aliases: [...initialAliases],
      parentId: parentId ?? null,
    });

    // Store the initial name and whether it was already an alias
    initialNameRef.current = initialName;
    initialNameWasAliasRef.current = initialAliases.some(
      (a) => a.toLowerCase() === initialName.toLowerCase(),
    );
  }, [open, entityName, aliases, parentId, parentName]);

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
        return [...prev, initName];
      }

      if (nameMovedBack && alreadyPresent && !initialNameWasAliasRef.current) {
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

  const currentParentId = parentCleared
    ? null
    : selectedParent
      ? selectedParent.id
      : (parentId ?? null);

  // Resolve pending alias input into the effective alias list for comparison
  const resolvedAliases = useMemo(
    () => resolveAliases(editAliases, aliasInput),
    [editAliases, aliasInput],
  );

  const hasChanges = useMemo(() => {
    if (changesSaved) return false;
    if (!initialValues) return false;
    const aliasesChanged =
      resolvedAliases.length !== initialValues.aliases.length ||
      resolvedAliases.some((a, i) => a !== initialValues.aliases[i]);
    const parentChanged = currentParentId !== initialValues.parentId;
    return name !== initialValues.name || aliasesChanged || parentChanged;
  }, [name, changesSaved, initialValues, resolvedAliases, currentParentId]);

  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

  const handleSubmit = async () => {
    setServerError(null);
    try {
      const data: PublisherEditData = {
        name,
        aliases: resolvedAliases,
      };
      // Only send parent_id if it changed
      if (initialValues && currentParentId !== initialValues.parentId) {
        data.parent_id = currentParentId;
      }
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
          <DialogTitle>Edit Publisher</DialogTitle>
          <DialogDescription>
            Update the publisher name, parent, and aliases.
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-6">
          <div className="space-y-2">
            <Label htmlFor="publisher-name">Name</Label>
            <Input
              id="publisher-name"
              onChange={(e) => handleNameChange(e.target.value)}
              value={name}
            />
          </div>

          <div className="space-y-2">
            <Label>Parent Publisher</Label>
            <div className="flex items-center gap-2">
              <div className="flex-1">
                <EntityCombobox<PublisherIdOption>
                  canCreate={false}
                  getOptionDescription={(p) =>
                    `${p.file_count} ${p.file_count === 1 ? "file" : "files"}`
                  }
                  getOptionKey={(item) => item.id}
                  getOptionLabel={(item) => item.name}
                  hook={useParentSearch}
                  label="Publisher"
                  onChange={(next) => {
                    if ("__create" in next) return;
                    setSelectedParent(next);
                    setParentCleared(false);
                  }}
                  placeholder="No parent (root publisher)"
                  value={parentCleared ? null : selectedParent}
                />
              </div>
              {(selectedParent || (parentId && !parentCleared)) && (
                <Button
                  aria-label="Clear parent publisher"
                  className="cursor-pointer shrink-0"
                  onClick={() => {
                    setSelectedParent(null);
                    setParentCleared(true);
                  }}
                  size="icon"
                  type="button"
                  variant="ghost"
                >
                  <X className="h-4 w-4" />
                </Button>
              )}
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="publisher-aliases">Aliases</Label>
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
                id="publisher-aliases"
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
