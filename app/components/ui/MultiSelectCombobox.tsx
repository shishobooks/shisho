import { ChevronsUpDown, Loader2, Plus, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";

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
import type { OptionSearch } from "@/hooks/queries/entity-search";
import { useDebounce } from "@/hooks/useDebounce";
import { useHighlightFirstOption } from "@/hooks/useHighlightFirstOption";
import { toastRequestError } from "@/libraries/api";

interface MultiSelectComboboxProps<T> {
  values: string[];
  onChange: (values: string[]) => void;
  hook: (query: string) => OptionSearch<T>;
  /** Names the search box ("Search genre"), and the trigger too unless
   * labelId is set. */
  label: string;
  /**
   * The id of the field's visible label, which then names the trigger, so
   * the name is the text a user sees. Without one, `label` names it.
   */
  labelId?: string;
  getOptionLabel: (item: T) => string;
  getOptionDescription?: (item: T) => string | undefined;
  getOptionCount?: (item: T) => number | undefined;
  placeholder?: string;
  useSelectedItemCounts?: (values: string[]) => Map<string, number>;
  /**
   * Whether a pressed Enter is still waiting for its search, so `values` is
   * about to change. A form holds its save until this turns false.
   */
  onPendingChange?: (pending: boolean) => void;
}

/**
 * Runs the search for one Enter pressed before its results arrived, apart from
 * the search behind the box, which has moved on to whatever is typed next.
 */
function EarlyEnterSearch<T>({
  text,
  hook,
  onAnswer,
  onFailure,
}: {
  text: string;
  hook: (query: string) => OptionSearch<T>;
  onAnswer: (items: T[]) => void;
  onFailure: (error: unknown) => void;
}) {
  const { data, isLoading, isPlaceholderData, isPaused, isError, error } =
    hook(text);
  const waiting = isLoading || isPlaceholderData || isPaused;
  // Strict Mode reruns the effect, and a second answer would add the next match.
  const finished = useRef(false);
  useEffect(() => {
    if (finished.current || (waiting && !isError)) return;
    finished.current = true;
    if (isError) onFailure(error);
    else onAnswer(data ?? []);
  });
  return null;
}

export function MultiSelectCombobox<T>({
  values,
  onChange,
  hook,
  label,
  labelId,
  getOptionLabel,
  getOptionDescription,
  getOptionCount,
  placeholder,
  useSelectedItemCounts,
  onPendingChange,
}: MultiSelectComboboxProps<T>) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebounce(search, 200);
  const [earlyEnters, setEarlyEnters] = useState<
    { id: number; text: string; valuesAtEnter: string[] }[]
  >([]);
  const nextEarlyEnterId = useRef(0);
  // Early Enters can answer in one render, so each adds to what the last added.
  const latestValues = useRef(values);
  latestValues.current = values;

  const hasEarlyEnters = earlyEnters.length > 0;
  useEffect(() => {
    onPendingChange?.(hasEarlyEnters);
    // Unmounting drops the wait, and a form must not stay held by it.
    return () => onPendingChange?.(false);
  }, [hasEarlyEnters, onPendingChange]);
  const {
    data: items = [],
    isLoading,
    isPlaceholderData,
    isError,
  } = hook(debouncedSearch);

  const chipCounts = useRef(new Map<string, number>());
  const rememberCount = (item: T) => {
    const count = getOptionCount?.(item);
    if (count != null) chipCounts.current.set(getOptionLabel(item), count);
  };
  items.forEach(rememberCount);

  const missingValues = values.filter((v) => !chipCounts.current.has(v));
  const selectedCounts = useSelectedItemCounts
    ? useSelectedItemCounts(missingValues)
    : undefined;
  if (selectedCounts) {
    for (const [name, count] of selectedCounts) {
      chipCounts.current.set(name, count);
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

  const addValue = (value: string) => {
    latestValues.current = [...latestValues.current, value];
    onChange(latestValues.current);
  };

  const hasValue = (value: string) =>
    latestValues.current.some((v) => v.toLowerCase() === value.toLowerCase());

  const handleSelect = (item: T) => {
    const itemLabel = getOptionLabel(item);
    if (!hasValue(itemLabel)) {
      addValue(itemLabel);
    }
    setSearch("");
  };

  const handleCreate = () => {
    if (trimmed && !hasValue(trimmed)) {
      addValue(trimmed);
    }
    setSearch("");
  };

  const handleEarlyEnter = () => {
    if (!trimmed) return;
    const id = nextEarlyEnterId.current++;
    setEarlyEnters((prev) => [
      ...prev,
      { id, text: trimmed, valuesAtEnter: latestValues.current },
    ]);
    setSearch("");
  };

  const finishEarlyEnter = (id: number) =>
    setEarlyEnters((prev) => prev.filter((enter) => enter.id !== id));

  // Picks what Enter would have, had the results for the text been showing:
  // the first match not yet chosen when it was pressed. If that match has been
  // chosen since (a click, the same text entered twice), it's already done, and
  // moving on to the next match would add a tag nobody asked for.
  const answerEarlyEnter = (
    {
      id,
      text,
      valuesAtEnter,
    }: { id: number; text: string; valuesAtEnter: string[] },
    answer: T[],
  ) => {
    finishEarlyEnter(id);
    const wasChosen = (name: string) =>
      valuesAtEnter.some((v) => v.toLowerCase() === name.toLowerCase());
    const first = answer.find((item) => !wasChosen(getOptionLabel(item)));
    const picked = first ? getOptionLabel(first) : text;
    if (first) rememberCount(first);
    if (!hasValue(picked)) addValue(picked);
  };

  const highlight = useHighlightFirstOption({
    options: [
      ...filtered.map((opt) => ({
        value: getOptionLabel(opt),
        select: () => handleSelect(opt),
      })),
      ...(showCreate
        ? [{ value: `create-${trimmed}`, select: handleCreate }]
        : []),
    ],
    loading: isLoading,
    // An empty box has no search to wait for, and an option moved to on it can
    // be picked as shown.
    pending: !!trimmed && (isPlaceholderData || search !== debouncedSearch),
    failed: isError,
    open,
    // An empty box asked for nothing, so Enter must not add the list's first
    // option. Only typing or moving to an option gives Enter something to pick.
    highlightFirst: !!trimmed,
    onEnterWhileWaiting: handleEarlyEnter,
  });

  const handleRemove = (value: string) => {
    onChange(values.filter((v) => v !== value));
  };

  return (
    <div className="space-y-2">
      {/* Outside the popover: a pressed Enter counts even if it closes. */}
      {earlyEnters.map((enter) => (
        <EarlyEnterSearch
          hook={hook}
          key={enter.id}
          onAnswer={(answer) => answerEarlyEnter(enter, answer)}
          onFailure={(error) => {
            finishEarlyEnter(enter.id);
            toastRequestError(error, `Failed to search for "${enter.text}"`);
          }}
          text={enter.text}
        />
      ))}
      {(values.length > 0 || hasEarlyEnters) && (
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
                      ({count.toLocaleString()})
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
          {earlyEnters.map(({ id, text }) => (
            <Badge
              className="flex items-center gap-1 max-w-full"
              data-testid="ms-pending-chip"
              key={id}
              variant="outline"
            >
              <Loader2 className="h-3 w-3 animate-spin shrink-0" />
              <span className="truncate">{text}</span>
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
            aria-label={labelId ? undefined : label}
            aria-labelledby={labelId}
            className="w-full justify-between"
            role="combobox"
            variant="outline"
          >
            {placeholder ?? `Add ${label.toLowerCase()}...`}
            <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-full p-0">
          <Command
            label={`Search ${label.toLowerCase()}`}
            shouldFilter={false}
            {...highlight}
          >
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
