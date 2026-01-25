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
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";

interface MultiSelectComboboxProps {
  values: string[];
  onChange: (values: string[]) => void;
  options: string[];
  onSearch: (query: string) => void;
  searchValue: string;
  placeholder?: string;
  isLoading?: boolean;
  label: string;
}

export function MultiSelectCombobox({
  values,
  onChange,
  options,
  onSearch,
  searchValue,
  placeholder = "Search...",
  isLoading = false,
  label,
}: MultiSelectComboboxProps) {
  const [open, setOpen] = useState(false);

  const handleSelect = (value: string) => {
    if (!values.includes(value)) {
      onChange([...values, value]);
    }
    onSearch("");
  };

  const handleCreate = () => {
    const trimmed = searchValue.trim();
    if (
      trimmed &&
      !values.some((v) => v.toLowerCase() === trimmed.toLowerCase())
    ) {
      onChange([...values, trimmed]);
    }
    onSearch("");
  };

  const handleRemove = (value: string) => {
    onChange(values.filter((v) => v !== value));
  };

  // Filter out already-selected values from options
  const filteredOptions = options.filter(
    (opt) => !values.some((v) => v.toLowerCase() === opt.toLowerCase()),
  );

  const showCreateOption =
    searchValue.trim() &&
    !filteredOptions.some(
      (opt) => opt.toLowerCase() === searchValue.toLowerCase(),
    ) &&
    !values.some((v) => v.toLowerCase() === searchValue.toLowerCase());

  return (
    <div className="space-y-2">
      {/* Selected values as badges */}
      {values.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {values.map((value) => (
            <Badge
              className="flex items-center gap-1"
              key={value}
              variant="secondary"
            >
              {value}
              <button
                className="ml-1 cursor-pointer hover:text-destructive"
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

      {/* Combobox */}
      <Popover modal onOpenChange={setOpen} open={open}>
        <PopoverTrigger asChild>
          <Button
            aria-expanded={open}
            className="w-full justify-between"
            role="combobox"
            variant="outline"
          >
            {placeholder}
            <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-full p-0">
          <Command shouldFilter={false}>
            <CommandInput
              onValueChange={onSearch}
              placeholder={`Search ${label.toLowerCase()}...`}
              value={searchValue}
            />
            <CommandList>
              {isLoading && (
                <div className="p-4 text-center text-sm text-muted-foreground">
                  Loading...
                </div>
              )}
              {!isLoading &&
                filteredOptions.length === 0 &&
                !showCreateOption && (
                  <div className="p-4 text-center text-sm text-muted-foreground">
                    {!searchValue
                      ? `No ${label.toLowerCase()} available. Type to create one.`
                      : `No matching ${label.toLowerCase()}.`}
                  </div>
                )}
              {!isLoading && (
                <CommandGroup>
                  {filteredOptions.map((opt) => (
                    <CommandItem
                      key={opt}
                      onSelect={() => handleSelect(opt)}
                      value={opt}
                    >
                      <Check className="mr-2 h-4 w-4 opacity-0" />
                      {opt}
                    </CommandItem>
                  ))}
                  {showCreateOption && (
                    <CommandItem
                      onSelect={handleCreate}
                      value={`create-${searchValue}`}
                    >
                      <Plus className="mr-2 h-4 w-4" />
                      Create "{searchValue}"
                    </CommandItem>
                  )}
                </CommandGroup>
              )}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  );
}
