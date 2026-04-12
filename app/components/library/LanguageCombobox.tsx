import { Check, ChevronsUpDown, Plus, X } from "lucide-react";
import { useMemo, useState } from "react";

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
  PopoverAnchor,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { getLanguageName, LANGUAGES } from "@/constants/languages";
import { useLibraryLanguages } from "@/hooks/queries/libraries";
import { cn } from "@/libraries/utils";

interface LanguageComboboxProps {
  value: string;
  onChange: (value: string) => void;
  libraryId?: number;
  disabled?: boolean;
}

export function LanguageCombobox({
  value,
  onChange,
  libraryId,
  disabled,
}: LanguageComboboxProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const { data: libraryLanguages } = useLibraryLanguages(libraryId);

  const mergedLanguages = useMemo(() => {
    const curatedTags = new Set(LANGUAGES.map((l) => l.tag));
    const extras: { tag: string; name: string }[] = [];
    if (libraryLanguages) {
      for (const tag of libraryLanguages) {
        if (!curatedTags.has(tag)) {
          extras.push({ tag, name: tag });
        }
      }
    }
    return [...LANGUAGES, ...extras];
  }, [libraryLanguages]);

  const filteredLanguages = useMemo(() => {
    if (!search.trim()) return mergedLanguages;
    const searchLower = search.trim().toLowerCase();
    return mergedLanguages.filter(
      (l) =>
        l.name.toLowerCase().includes(searchLower) ||
        l.tag.toLowerCase().includes(searchLower),
    );
  }, [mergedLanguages, search]);

  const showCustomOption = useMemo(() => {
    if (!search.trim()) return false;
    const searchLower = search.trim().toLowerCase();
    return !mergedLanguages.some(
      (l) =>
        l.tag.toLowerCase() === searchLower ||
        l.name.toLowerCase() === searchLower,
    );
  }, [search, mergedLanguages]);

  const handleSelect = (tag: string) => {
    onChange(tag);
    setOpen(false);
    setSearch("");
  };

  const handleCreate = () => {
    if (search.trim()) onChange(search.trim());
    setOpen(false);
    setSearch("");
  };

  const handleClear = () => onChange("");

  const displayName = getLanguageName(value);
  const badgeLabel = displayName ? `${displayName} (${value})` : value;

  return (
    <Popover modal onOpenChange={disabled ? undefined : setOpen} open={open}>
      {value ? (
        <PopoverAnchor asChild>
          <div className="flex items-center gap-2">
            <Badge
              className={cn(
                "flex items-center gap-1 max-w-full",
                !disabled && "cursor-pointer",
              )}
              onClick={() => {
                if (!disabled) setOpen(true);
              }}
              variant="secondary"
            >
              <span className="truncate" title={badgeLabel}>
                {badgeLabel}
              </span>
            </Badge>
            {!disabled && (
              <button
                aria-label="Clear language"
                className="cursor-pointer text-muted-foreground hover:text-destructive shrink-0"
                onClick={handleClear}
                type="button"
              >
                <X className="h-3 w-3" />
              </button>
            )}
          </div>
        </PopoverAnchor>
      ) : (
        <PopoverTrigger asChild>
          <Button
            aria-expanded={open}
            className="w-full justify-between"
            disabled={disabled}
            role="combobox"
            variant="outline"
          >
            Select language...
            <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
      )}
      <PopoverContent align="start" className="w-full p-0">
        <Command shouldFilter={false}>
          <CommandInput
            onValueChange={setSearch}
            placeholder="Search languages..."
            value={search}
          />
          <CommandList>
            {filteredLanguages.length === 0 && !showCustomOption && (
              <div className="p-4 text-center text-sm text-muted-foreground">
                No matching languages.
              </div>
            )}
            <CommandGroup>
              {filteredLanguages.map((l) => (
                <CommandItem
                  key={l.tag}
                  onSelect={() => handleSelect(l.tag)}
                  value={l.tag}
                >
                  <Check
                    className={cn(
                      "mr-2 h-4 w-4 shrink-0",
                      value === l.tag ? "opacity-100" : "opacity-0",
                    )}
                  />
                  <span className="truncate" title={l.name}>
                    {l.name}
                  </span>
                  <span className="ml-auto text-xs text-muted-foreground shrink-0">
                    {l.tag}
                  </span>
                </CommandItem>
              ))}
              {showCustomOption && (
                <CommandItem onSelect={handleCreate} value={`create-${search}`}>
                  <Plus className="mr-2 h-4 w-4 shrink-0" />
                  <span className="truncate">
                    Use custom tag: &quot;{search}&quot;
                  </span>
                </CommandItem>
              )}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
