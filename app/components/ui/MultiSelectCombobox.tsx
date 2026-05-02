import { ChevronsUpDown, Plus, X } from "lucide-react";
import { useRef, useState } from "react";

import { Badge } from "@/components/ui/badge";
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

interface MultiSelectComboboxProps<T> {
  values: string[];
  onChange: (values: string[]) => void;
  hook: (query: string) => { data?: T[]; isLoading: boolean };
  label: string;
  getOptionLabel: (item: T) => string;
  getOptionDescription?: (item: T) => string | undefined;
  getOptionCount?: (item: T) => number | undefined;
  placeholder?: string;
}

export function MultiSelectCombobox<T>({
  values,
  onChange,
  hook,
  label,
  getOptionLabel,
  getOptionDescription,
  getOptionCount,
  placeholder,
}: MultiSelectComboboxProps<T>) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebounce(search, 200);
  const { data: items = [], isLoading } = hook(debouncedSearch);

  const chipCounts = useRef(new Map<string, number>());
  if (getOptionCount) {
    for (const item of items) {
      const c = getOptionCount(item);
      if (c != null) chipCounts.current.set(getOptionLabel(item), c);
    }
  }

  const filtered = items.filter(
    (opt) =>
      !values.some(
        (v) => v.toLowerCase() === getOptionLabel(opt).toLowerCase(),
      ),
  );

  const trimmed = search.trim();
  const showCreate =
    !!trimmed &&
    !filtered.some(
      (opt) => getOptionLabel(opt).toLowerCase() === trimmed.toLowerCase(),
    ) &&
    !values.some((v) => v.toLowerCase() === trimmed.toLowerCase());

  const handleSelect = (item: T) => {
    const itemLabel = getOptionLabel(item);
    if (!values.includes(itemLabel)) {
      onChange([...values, itemLabel]);
    }
    setSearch("");
  };

  const handleCreate = () => {
    if (
      trimmed &&
      !values.some((v) => v.toLowerCase() === trimmed.toLowerCase())
    ) {
      onChange([...values, trimmed]);
    }
    setSearch("");
  };

  const handleRemove = (value: string) => {
    onChange(values.filter((v) => v !== value));
  };

  return (
    <div className="space-y-2">
      {values.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          {values.map((value) => {
            const count = chipCounts.current.get(value);
            return (
              <Badge
                className="flex items-center gap-1 max-w-full"
                data-testid="ms-chip"
                key={value}
                variant="secondary"
              >
                <span className="truncate" title={value}>
                  {value}
                  {count != null && (
                    <span className="text-muted-foreground ml-1">
                      ({count})
                    </span>
                  )}
                </span>
                <button
                  aria-label={`Remove ${value}`}
                  className="ml-1 cursor-pointer hover:text-destructive shrink-0"
                  onClick={() => handleRemove(value)}
                  type="button"
                >
                  <X className="h-3 w-3" />
                </button>
              </Badge>
            );
          })}
          {values.length > 1 && (
            <button
              className="text-xs text-muted-foreground hover:text-destructive cursor-pointer"
              onClick={() => onChange([])}
              type="button"
            >
              Clear all
            </button>
          )}
        </div>
      )}

      <Popover modal onOpenChange={setOpen} open={open}>
        <PopoverTrigger asChild>
          <Button
            aria-expanded={open}
            className="w-full justify-between"
            role="combobox"
            variant="outline"
          >
            {placeholder ?? `Add ${label.toLowerCase()}...`}
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
                  {!trimmed
                    ? `No ${label.toLowerCase()} available. Type to create one.`
                    : `No matching ${label.toLowerCase()}.`}
                </div>
              )}
              {!isLoading && filtered.length > 0 && (
                <CommandGroup heading="In your library">
                  {filtered.map((opt) => {
                    const optLabel = getOptionLabel(opt);
                    return (
                      <CommandItem
                        className="cursor-pointer"
                        key={optLabel}
                        onSelect={() => handleSelect(opt)}
                        value={optLabel}
                      >
                        <span className="truncate">{optLabel}</span>
                        {getOptionDescription?.(opt) && (
                          <span className="ml-auto pl-2 text-xs text-muted-foreground shrink-0">
                            {getOptionDescription(opt)}
                          </span>
                        )}
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
                      onSelect={handleCreate}
                      value={`create-${trimmed}`}
                    >
                      <Plus className="mr-2 h-4 w-4" />
                      Create new {label.toLowerCase()} &quot;{trimmed}&quot;
                    </CommandItem>
                  </CommandGroup>
                </>
              )}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  );
}
