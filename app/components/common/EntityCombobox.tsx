import { Check, ChevronsUpDown, Plus } from "lucide-react";
import { useState } from "react";

import {
  StatusBadge,
  type EntityStatus,
} from "@/components/common/StatusBadge";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { useDebounce } from "@/hooks/useDebounce";

export type { EntityStatus };

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
  placeholder,
}: EntityComboboxProps<T>) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  // Debounce the query before fanning out to the consumer's hook so
  // typing-fast doesn't fire one query per keystroke. The pre-refactor
  // edit-dialog blocks debounced at the parent level via useDebounce; we
  // hold that contract here so callers don't need to repeat the pattern.
  const debouncedSearch = useDebounce(search, 200);
  const { data: items = [], isLoading } = hook(debouncedSearch);

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
      ? (placeholder ?? `Add ${label.toLowerCase()}`)
      : "__create" in value
        ? value.__create
        : getOptionLabel(value);

  return (
    <div className="flex items-center gap-2">
      <Popover modal onOpenChange={setOpen} open={open}>
        <PopoverTrigger asChild>
          <Button
            aria-expanded={open}
            className="w-full justify-between cursor-pointer"
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
              {!isLoading && filtered.length > 0 && (
                <CommandGroup heading="In your library">
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
                </CommandGroup>
              )}
              {!isLoading && showCreate && (
                <>
                  {filtered.length > 0 && <CommandSeparator />}
                  <CommandGroup>
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
                      <span className="truncate">
                        Create new {label.toLowerCase()} &quot;{trimmed}&quot;
                      </span>
                    </CommandItem>
                  </CommandGroup>
                </>
              )}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
      {status && <StatusBadge status={status} />}
    </div>
  );
}
