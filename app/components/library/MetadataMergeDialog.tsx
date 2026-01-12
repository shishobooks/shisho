import type { EntityType } from "./MetadataEditDialog";
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

interface EntityOption {
  id: number;
  name: string;
  count: number;
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
}

const ENTITY_PLURALS: Record<EntityType, string> = {
  person: "people",
  series: "series",
  genre: "genres",
  tag: "tags",
  publisher: "publishers",
  imprint: "imprints",
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
}: MetadataMergeDialogProps) {
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [comboboxOpen, setComboboxOpen] = useState(false);
  const [search, setSearch] = useState("");

  // Filter out the target entity from the list
  const availableEntities = useMemo(() => {
    return entities.filter((e) => e.id !== targetId);
  }, [entities, targetId]);

  const selectedEntity = availableEntities.find((e) => e.id === selectedId);

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

  const handleOpenChange = (isOpen: boolean) => {
    if (!isOpen) {
      setSelectedId(null);
      setSearch("");
    }
    onOpenChange(isOpen);
  };

  return (
    <Dialog onOpenChange={handleOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Merge into "{targetName}"</DialogTitle>
          <DialogDescription>
            Select a {entityType} to merge into this one. All associated books
            will be transferred and the selected {entityType} will be deleted.
          </DialogDescription>
        </DialogHeader>

        <div className="py-4">
          <Popover modal onOpenChange={setComboboxOpen} open={comboboxOpen}>
            <PopoverTrigger asChild>
              <Button
                aria-expanded={comboboxOpen}
                className="w-full justify-between"
                role="combobox"
                variant="outline"
              >
                {selectedEntity
                  ? `${selectedEntity.name} (${selectedEntity.count} books)`
                  : `Select ${entityType} to merge...`}
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
                            className={`mr-2 h-4 w-4 ${
                              selectedId === entity.id
                                ? "opacity-100"
                                : "opacity-0"
                            }`}
                          />
                          <span className="flex-1">{entity.name}</span>
                          <span className="text-muted-foreground text-sm">
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

          {selectedEntity && (
            <p className="mt-4 text-sm text-muted-foreground">
              This will move all {selectedEntity.count} book
              {selectedEntity.count !== 1 ? "s" : ""} from "
              {selectedEntity.name}" to "{targetName}" and delete "
              {selectedEntity.name}".
            </p>
          )}
        </div>

        <DialogFooter>
          <Button onClick={() => handleOpenChange(false)} variant="outline">
            Cancel
          </Button>
          <Button
            disabled={isPending || !selectedId}
            onClick={handleMerge}
            variant="destructive"
          >
            {isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Merge
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
