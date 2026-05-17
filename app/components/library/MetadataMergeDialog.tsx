import { Check, ChevronsUpDown, Loader2 } from "lucide-react";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

import type { EntityType } from "./MetadataEditDialog";

interface EntityOption {
  id: number;
  name: string;
  count: number;
}

interface SetChildConfig {
  onSetChild: (childId: number) => Promise<void>;
  isPending: boolean;
  /** IDs of entities that are ancestors of the target — setting them as child would create a cycle */
  disabledIds: number[];
}

interface MetadataMergeDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: EntityType;
  targetName: string;
  targetId: number;
  onMerge: (sourceId: number) => Promise<void>;
  isPending: boolean;
  entities: EntityOption[];
  isLoadingEntities: boolean;
  onSearch: (search: string) => void;
  /** When provided, shows a "Set as child" option alongside Merge */
  setChildConfig?: SetChildConfig;
}

const ENTITY_PLURALS: Record<EntityType, string> = {
  person: "people",
  series: "series",
  genre: "genres",
  tag: "tags",
  publisher: "publishers",
};

export function MetadataMergeDialog({
  open,
  onOpenChange,
  entityType,
  targetName,
  targetId,
  onMerge,
  isPending,
  entities,
  isLoadingEntities,
  onSearch,
  setChildConfig,
}: MetadataMergeDialogProps) {
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [comboboxOpen, setComboboxOpen] = useState(false);
  const [search, setSearch] = useState("");

  // Filter out the target entity from the list
  const availableEntities = useMemo(() => {
    return entities.filter((e) => e.id !== targetId);
  }, [entities, targetId]);

  const selectedEntity = availableEntities.find((e) => e.id === selectedId);

  // Determine if "Set as child" is disabled for the selected entity (would create cycle)
  const setChildDisabled = useMemo(() => {
    if (!setChildConfig || !selectedId) return false;
    return setChildConfig.disabledIds.includes(selectedId);
  }, [setChildConfig, selectedId]);

  const handleSearchChange = (value: string) => {
    setSearch(value);
    onSearch(value);
  };

  const handleMerge = async () => {
    if (selectedId) {
      await onMerge(selectedId);
      setSelectedId(null);
      setSearch("");
    }
  };

  const handleSetChild = async () => {
    if (selectedId && setChildConfig) {
      await setChildConfig.onSetChild(selectedId);
      setSelectedId(null);
      setSearch("");
    }
  };

  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      setSelectedId(null);
      setSearch("");
    }
    onOpenChange(isOpen);
  };

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent className="max-w-xl overflow-x-hidden">
        <DialogHeader>
          <DialogTitle title={`Merge into "${targetName}"`}>
            Merge into "{targetName}"
          </DialogTitle>
          <DialogDescription>
            Select a {entityType} to merge into this one.{" "}
            {setChildConfig
              ? "Then choose to merge or set as a child."
              : `All associated books will be transferred and the selected ${entityType} will be deleted.`}
          </DialogDescription>
        </DialogHeader>

        <DialogBody>
          <Popover modal onOpenChange={setComboboxOpen} open={comboboxOpen}>
            <PopoverTrigger asChild>
              <Button
                aria-expanded={comboboxOpen}
                className="w-full justify-between min-w-0"
                role="combobox"
                variant="outline"
              >
                <span className="truncate">
                  {selectedEntity
                    ? `${selectedEntity.name} (${selectedEntity.count} ${selectedEntity.count !== 1 ? "files" : "file"})`
                    : `Select ${entityType}...`}
                </span>
                <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
              </Button>
            </PopoverTrigger>
            <PopoverContent align="start" className="w-full p-0">
              <Command shouldFilter={false}>
                <CommandInput
                  onValueChange={handleSearchChange}
                  placeholder={`Search ${ENTITY_PLURALS[entityType]}...`}
                  value={search}
                />
                <CommandList>
                  {isLoadingEntities && (
                    <div className="p-4 text-center text-sm text-muted-foreground">
                      Loading...
                    </div>
                  )}
                  {!isLoadingEntities && availableEntities.length === 0 && (
                    <div className="p-4 text-center text-sm text-muted-foreground">
                      No {ENTITY_PLURALS[entityType]} found
                    </div>
                  )}
                  {!isLoadingEntities && availableEntities.length > 0 && (
                    <CommandGroup>
                      {availableEntities.map((entity) => (
                        <CommandItem
                          key={entity.id}
                          onSelect={() => {
                            setSelectedId(entity.id);
                            setComboboxOpen(false);
                          }}
                          value={String(entity.id)}
                        >
                          <Check
                            className={`mr-2 h-4 w-4 shrink-0 ${
                              selectedId === entity.id
                                ? "opacity-100"
                                : "opacity-0"
                            }`}
                          />
                          <span className="flex-1 truncate" title={entity.name}>
                            {entity.name}
                          </span>
                          <span className="text-muted-foreground text-sm shrink-0 ml-2">
                            {entity.count} book{entity.count !== 1 ? "s" : ""}
                          </span>
                        </CommandItem>
                      ))}
                    </CommandGroup>
                  )}
                </CommandList>
              </Command>
            </PopoverContent>
          </Popover>

          {selectedEntity && !setChildConfig && (
            <p className="mt-4 text-sm text-muted-foreground">
              This will move all {selectedEntity.count} book
              {selectedEntity.count !== 1 ? "s" : ""} from "
              {selectedEntity.name}" to "{targetName}" and delete "
              {selectedEntity.name}".
            </p>
          )}

          {selectedEntity && setChildConfig && (
            <div className="mt-4 space-y-3">
              <div className="rounded-md border p-3">
                <p className="text-sm font-medium mb-1">Merge</p>
                <p className="text-xs text-muted-foreground">
                  Move all files from "{selectedEntity.name}" to "{targetName}",
                  add "{selectedEntity.name}" as an alias, and delete "
                  {selectedEntity.name}".
                </p>
              </div>
              <div className="rounded-md border p-3">
                <p className="text-sm font-medium mb-1">Set as child</p>
                <p className="text-xs text-muted-foreground">
                  Make "{selectedEntity.name}" a child of "{targetName}". Both
                  publishers keep their files and identity.
                </p>
                {setChildDisabled && (
                  <p className="text-xs text-destructive mt-1">
                    Cannot set as child: "{selectedEntity.name}" is already an
                    ancestor of "{targetName}", which would create a cycle.
                  </p>
                )}
              </div>
            </div>
          )}
        </DialogBody>

        <DialogFooter>
          <Button
            onClick={() => handleOpenChange(false)}
            size="sm"
            variant="outline"
          >
            Cancel
          </Button>
          {setChildConfig && (
            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <Button
                    disabled={
                      setChildConfig.isPending ||
                      !selectedId ||
                      setChildDisabled
                    }
                    onClick={handleSetChild}
                    size="sm"
                    variant="secondary"
                  >
                    {setChildConfig.isPending && (
                      <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
                    )}
                    Set as child
                  </Button>
                </span>
              </TooltipTrigger>
              {setChildDisabled && (
                <TooltipContent>
                  Would create a cycle in the publisher hierarchy
                </TooltipContent>
              )}
            </Tooltip>
          )}
          <Button
            disabled={isPending || !selectedId}
            onClick={handleMerge}
            size="sm"
            variant="destructive"
          >
            {isPending && <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />}
            Merge
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
