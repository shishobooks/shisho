import { Check, ChevronsUpDown, Plus, X } from "lucide-react";
import { useMemo, useRef, useState } from "react";

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
  /** Names the picker ("Language") unless labelId is set; its text shows
   * the prompt instead. */
  label: string;
  /**
   * The id of the field's visible label, which then names the trigger, so
   * the name is the text a user sees. Without one, `label` names it.
   */
  labelId?: string;
  value: string;
  onChange: (value: string) => void;
  libraryId?: number;
  disabled?: boolean;
}

export function LanguageCombobox({
  label,
  labelId,
  value,
  onChange,
  libraryId,
  disabled,
}: LanguageComboboxProps) {
  const [open, setOpen] = useState(false);
  // The language button opens the picker through a PopoverAnchor, not a
  // trigger, so Radix cannot return focus to it on close.
  const badgeButtonRef = useRef<HTMLButtonElement>(null);
  const [search, setSearch] = useState("");
  // The languages route needs Books Read, which the hook checks. Without it
  // the curated list and custom tag entry still work, just without the
  // library's own tags.
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
  const badgeText = (
    <span className="truncate" title={badgeLabel}>
      {badgeLabel}
    </span>
  );

  return (
    <Popover modal onOpenChange={disabled ? undefined : setOpen} open={open}>
      {value ? (
        <PopoverAnchor asChild>
          <div className="flex items-center gap-2">
            {/* Clicking the language reopens the picker, so it is a button
                unless the field is disabled. */}
            <Badge
              asChild={!disabled}
              className="flex items-center gap-1 max-w-full"
              variant="secondary"
            >
              {disabled ? (
                badgeText
              ) : (
                <Button
                  aria-label={`Change language: ${badgeLabel}`}
                  onClick={() => setOpen(true)}
                  ref={badgeButtonRef}
                  variant="unstyled"
                >
                  {badgeText}
                </Button>
              )}
            </Badge>
            {!disabled && (
              <Button
                aria-label="Clear language"
                className="shrink-0 text-muted-foreground hover:text-destructive"
                onClick={handleClear}
                variant="unstyled"
              >
                <X className="h-3 w-3" />
              </Button>
            )}
          </div>
        </PopoverAnchor>
      ) : (
        <PopoverTrigger asChild>
          <Button
            aria-expanded={open}
            aria-label={labelId ? undefined : label}
            aria-labelledby={labelId}
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
      <PopoverContent
        align="start"
        className="w-full p-0"
        onCloseAutoFocus={(e) => {
          if (!badgeButtonRef.current) return;
          e.preventDefault();
          badgeButtonRef.current.focus();
        }}
      >
        <Command label="Search languages" shouldFilter={false}>
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
                  {/* cmdk's aria-selected marks the highlighted option, so
                      the chosen language is spoken as text. */}
                  {value === l.tag && <span className="sr-only">(chosen)</span>}
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
