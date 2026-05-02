import { Check, ChevronsUpDown, Plus, X } from "lucide-react";
import { useState } from "react";

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

interface MultiSelectComboboxProps {
  values: string[];
  onChange: (values: string[]) => void;
  hook: (query: string) => { data?: string[]; isLoading: boolean };
  label: string;
  placeholder?: string;
}

export function MultiSelectCombobox({
  values,
  onChange,
  hook,
  label,
  placeholder,
}: MultiSelectComboboxProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebounce(search, 200);
  const { data: items = [], isLoading } = hook(debouncedSearch);

  const filtered = items.filter(
    (opt) => !values.some((v) => v.toLowerCase() === opt.toLowerCase()),
  );

  const trimmed = search.trim();
  const showCreate =
    !!trimmed &&
    !filtered.some((opt) => opt.toLowerCase() === trimmed.toLowerCase()) &&
    !values.some((v) => v.toLowerCase() === trimmed.toLowerCase());

  const handleSelect = (value: string) => {
    if (!values.includes(value)) {
      onChange([...values, value]);
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
          {values.map((value) => (
            <Badge
              className="flex items-center gap-1 max-w-full"
              data-testid="ms-chip"
              key={value}
              variant="secondary"
            >
              <span className="truncate" title={value}>
                {value}
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
          ))}
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
                  {filtered.map((opt) => (
                    <CommandItem
                      className="cursor-pointer"
                      key={opt}
                      onSelect={() => handleSelect(opt)}
                      value={opt}
                    >
                      <Check className="mr-2 h-4 w-4 opacity-0" />
                      {opt}
                    </CommandItem>
                  ))}
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
