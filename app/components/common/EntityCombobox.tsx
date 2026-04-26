import { Check, ChevronsUpDown, Plus } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/libraries/utils";

export type EntityStatus = "new" | "changed" | "unchanged";

export interface EntityComboboxProps<T extends object> {
  hook: (query: string) => { data?: T[]; isLoading: boolean };
  label: string;
  value: T | { __create: string } | null;
  onChange: (next: T | { __create: string }) => void;
  getOptionLabel: (item: T) => string;
  getOptionKey?: (item: T) => string | number;
  canCreate?: boolean;
  exclude?: (item: T) => boolean;
  status?: EntityStatus;
  pendingCreate?: boolean;
  placeholder?: string;
}

export function EntityCombobox<T extends object>({
  hook,
  label,
  value,
  onChange,
  getOptionLabel,
  getOptionKey,
  canCreate = true,
  exclude,
  status,
  pendingCreate,
  placeholder,
}: EntityComboboxProps<T>) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const { data: items = [], isLoading } = hook(search);

  const filtered = exclude ? items.filter((i) => !exclude(i)) : items;
  const trimmed = search.trim();
  const showCreate =
    canCreate &&
    !!trimmed &&
    !filtered.some(
      (i) => getOptionLabel(i).toLowerCase() === trimmed.toLowerCase(),
    );

  const triggerLabel =
    value == null
      ? (placeholder ?? `Add ${label.toLowerCase()}...`)
      : "__create" in value
        ? value.__create
        : getOptionLabel(value);

  return (
    <div className="flex items-center gap-2">
      <Popover modal onOpenChange={setOpen} open={open}>
        <PopoverTrigger asChild>
          <Button
            aria-expanded={open}
            className={cn(
              "w-full justify-between cursor-pointer",
              pendingCreate && "border-dashed",
            )}
            role="combobox"
            variant="outline"
          >
            <span className="truncate">{triggerLabel}</span>
            <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-full p-0">
          <Command shouldFilter={false}>
            <CommandInput
              onValueChange={setSearch}
              placeholder={`Search ${label.toLowerCase()}...`}
              value={search}
            />
            <CommandList>
              {isLoading && (
                <div className="p-4 text-center text-sm text-muted-foreground">
                  Loading...
                </div>
              )}
              {!isLoading && filtered.length === 0 && !showCreate && (
                <div className="p-4 text-center text-sm text-muted-foreground">
                  {trimmed
                    ? `No matching ${label.toLowerCase()}.`
                    : `No ${label.toLowerCase()} available.${canCreate ? " Type to create one." : ""}`}
                </div>
              )}
              {!isLoading && (
                <CommandGroup>
                  {filtered.map((item) => {
                    const key = getOptionKey
                      ? getOptionKey(item)
                      : getOptionLabel(item);
                    return (
                      <CommandItem
                        className="cursor-pointer"
                        key={key}
                        onSelect={() => {
                          onChange(item);
                          setOpen(false);
                          setSearch("");
                        }}
                        value={getOptionLabel(item)}
                      >
                        <Check className="mr-2 h-4 w-4 shrink-0 opacity-0" />
                        <span className="truncate">{getOptionLabel(item)}</span>
                      </CommandItem>
                    );
                  })}
                  {showCreate && (
                    <CommandItem
                      className="cursor-pointer"
                      onSelect={() => {
                        onChange({ __create: trimmed });
                        setOpen(false);
                        setSearch("");
                      }}
                      value={`create-${trimmed}`}
                    >
                      <Plus className="mr-2 h-4 w-4 shrink-0" />
                      <span className="truncate">Create "{trimmed}"</span>
                    </CommandItem>
                  )}
                </CommandGroup>
              )}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
      {status && (
        <Badge
          className={cn(
            status === "new" && "bg-green-600",
            status === "changed" && "bg-amber-600",
            status === "unchanged" && "bg-muted text-muted-foreground",
          )}
          data-testid="entity-status-badge"
          variant="default"
        >
          {status}
        </Badge>
      )}
    </div>
  );
}
