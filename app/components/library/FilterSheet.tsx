import * as SliderPrimitive from "@radix-ui/react-slider";
import {
  Bookmark,
  BookOpenCheck,
  ChevronsUpDown,
  Eye,
  File,
  Languages,
  ListFilter,
  Square,
  SquareCheckBig,
  Star,
  Tags,
} from "lucide-react";
import { forwardRef, useEffect, useId, useState, type ReactNode } from "react";

import {
  FilterChip,
  type SelectedFilter,
} from "@/components/library/FilterChip";
import QueryError, {
  type RetryableQuery,
} from "@/components/library/QueryError";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@/components/ui/drawer";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import {
  ratingRangeLabel,
  READING_STATUS_FILTER_OPTIONS,
  type ReadingFilters,
} from "@/libraries/readingFilters";
import { cn } from "@/libraries/utils";
import type { Genre, Tag } from "@/types";
import { formatCount } from "@/utils/format";
import { splitTagNamespace, type TagScope } from "@/utils/tagNamespaces";

type FilterOptionsQuery = RetryableQuery & { data: unknown };

interface FilterSheetProps {
  /** The reading filters, which read the signed-in user's own reading. */
  reading: ReadingFilters;
  onReadingChange: (next: ReadingFilters) => void;
  selectedFileTypes: string[];
  fileTypeOptions: readonly { value: string; label: string }[];
  onToggleFileType: (fileType: string) => void;
  selectedGenreIds: number[];
  selectedGenres: SelectedFilter[];
  genres: Genre[];
  genresLoading: boolean;
  /** The genres query, for its error report and Retry. */
  genresQuery: FilterOptionsQuery;
  genreSearchInput: string;
  onGenreSearchChange: (value: string) => void;
  onToggleGenre: (genreId: number) => void;
  selectedTagIds: number[];
  selectedTags: SelectedFilter[];
  tags: Tag[];
  tagsLoading: boolean;
  /** The tags query, for its error report and Retry. */
  tagsQuery: FilterOptionsQuery;
  tagSearchInput: string;
  onTagSearchChange: (value: string) => void;
  onToggleTag: (tagId: number) => void;
  /**
   * The library's tag namespaces. When there are any, the tag picker can
   * narrow to the plain tags or to one namespace.
   */
  tagNamespaces: readonly string[];
  tagScope: TagScope;
  onTagScopeChange: (scope: TagScope) => void;
  languageParam: string;
  languageOptions: { value: string; label: string }[];
  onLanguageChange: (value: string) => void;
  reviewedFilter: string;
  onReviewedFilterChange: (value: string) => void;
  onClearAll: () => void;
  hasActiveFilters: boolean;
}

const SectionHeader = ({
  icon,
  colorClass,
  label,
  labelId,
  detail,
}: {
  icon: React.ReactNode;
  colorClass: string;
  label: string;
  /** Lets the section's control name itself with aria-labelledby. */
  labelId?: string;
  detail?: string;
}) => (
  <div className="flex items-center gap-2 mb-3">
    <span
      className={cn(
        "inline-flex h-5 w-5 items-center justify-center rounded-sm",
        colorClass,
      )}
    >
      {icon}
    </span>
    <div className="text-xs font-semibold" id={labelId}>
      {label}
    </div>
    {detail && (
      <span className="ml-auto text-[10px] text-muted-foreground">
        {detail}
      </span>
    )}
  </div>
);

interface FilterComboboxOption {
  id: number;
  name: string;
  book_count: number;
}

const FilterCombobox = ({
  chipKind,
  selected,
  options,
  loading,
  query,
  errorFallback,
  searchInput,
  onSearchChange,
  onToggle,
  label,
  labelId,
  triggerLabel,
  searchPlaceholder,
  emptyText,
  header,
  optionLabel = (opt) => opt.name,
}: {
  chipKind: "genre" | "tag";
  selected: SelectedFilter[];
  options: FilterComboboxOption[];
  loading: boolean;
  query: FilterOptionsQuery;
  errorFallback: string;
  searchInput: string;
  onSearchChange: (value: string) => void;
  onToggle: (id: number) => void;
  /** Names the search box ("Search genres"). */
  label: string;
  /** The id of the section header, which names the trigger. */
  labelId: string;
  triggerLabel: string;
  searchPlaceholder: string;
  emptyText: string;
  /** Controls between the search box and the options. */
  header?: ReactNode;
  /** What an option shows; its name by default. */
  optionLabel?: (option: FilterComboboxOption) => ReactNode;
}) => {
  const [open, setOpen] = useState(false);
  const selectedIds = new Set(selected.map((s) => s.id));

  return (
    <div className="space-y-2">
      {selected.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {selected.map((item) => (
            <FilterChip
              key={item.id}
              kind={chipKind}
              label={item.name}
              onRemove={() => onToggle(item.id)}
            />
          ))}
        </div>
      )}
      <Popover modal onOpenChange={setOpen} open={open}>
        <PopoverTrigger asChild>
          <Button
            aria-expanded={open}
            aria-labelledby={labelId}
            className="w-full justify-between font-normal text-muted-foreground"
            role="combobox"
            variant="outline"
          >
            {triggerLabel}
            <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent
          align="start"
          className="w-[var(--radix-popover-trigger-width)] p-0"
        >
          <Command label={`Search ${label.toLowerCase()}`} shouldFilter={false}>
            <CommandInput
              onValueChange={onSearchChange}
              placeholder={searchPlaceholder}
              value={searchInput}
            />
            {header}
            <CommandList>
              {loading ? (
                <div className="py-6 text-center text-sm text-muted-foreground">
                  Loading...
                </div>
              ) : query.error && !query.data ? (
                <QueryError
                  className="m-1"
                  fallback={errorFallback}
                  query={query}
                />
              ) : options.length === 0 ? (
                <CommandEmpty>{emptyText}</CommandEmpty>
              ) : (
                <CommandGroup>
                  {options.map((opt) => (
                    <CommandItem
                      key={opt.id}
                      onSelect={() => onToggle(opt.id)}
                      value={opt.name}
                    >
                      {selectedIds.has(opt.id) ? (
                        <SquareCheckBig className="mr-2 h-4 w-4" />
                      ) : (
                        <Square className="mr-2 h-4 w-4" />
                      )}
                      {optionLabel(opt)}
                      {/* cmdk's aria-selected marks the highlighted option,
                          so the chosen state is spoken as text. */}
                      {selectedIds.has(opt.id) && (
                        <span className="sr-only">(chosen)</span>
                      )}
                      <span className="ml-auto text-xs text-muted-foreground">
                        {opt.book_count.toLocaleString()}
                      </span>
                    </CommandItem>
                  ))}
                </CommandGroup>
              )}
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  );
};

/**
 * A combobox of a few fixed options, any number of them chosen. The trigger
 * names the chosen ones, so the sheet stays short however many there are.
 */
const MultiSelectCombobox = <V extends string>({
  labelId,
  options,
  selected,
  onToggle,
  placeholder,
}: {
  /** The id of the section header, which names the trigger. */
  labelId: string;
  options: readonly { value: V; label: string }[];
  selected: readonly V[];
  onToggle: (value: V) => void;
  /** What the trigger says with nothing chosen. */
  placeholder: string;
}) => {
  const [open, setOpen] = useState(false);
  const chosen = options.filter((o) => selected.includes(o.value));
  return (
    <Popover modal onOpenChange={setOpen} open={open}>
      <PopoverTrigger asChild>
        <Button
          aria-expanded={open}
          aria-labelledby={labelId}
          className={cn(
            "w-full justify-between font-normal",
            chosen.length === 0 && "text-muted-foreground",
          )}
          role="combobox"
          variant="outline"
        >
          <span className="truncate">
            {chosen.length > 0
              ? chosen.map((o) => o.label).join(", ")
              : placeholder}
          </span>
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-[var(--radix-popover-trigger-width)] p-0"
      >
        <Command shouldFilter={false}>
          <CommandList>
            <CommandGroup>
              {options.map((option) => {
                const isSelected = selected.includes(option.value);
                return (
                  <CommandItem
                    key={option.value}
                    onSelect={() => onToggle(option.value)}
                    value={option.value}
                  >
                    {isSelected ? (
                      <SquareCheckBig className="mr-2 h-4 w-4" />
                    ) : (
                      <Square className="mr-2 h-4 w-4" />
                    )}
                    {option.label}
                    {/* cmdk's aria-selected marks the highlighted option,
                        so the chosen state is spoken as text. */}
                    {isSelected && <span className="sr-only">(chosen)</span>}
                  </CommandItem>
                );
              })}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
};

// The rating slider's ends. Each end at its edge of the track sets no bound.
const RATING_FLOOR = 0;
const RATING_CEILING = 5;

const ratingEndText = (stars: number, edge: number) =>
  stars === edge ? "Any" : formatCount(stars, "star");

/**
 * The rating range as a slider with a thumb for each end, in quarter stars.
 * Either thumb at its edge of the track leaves that end unbounded, so the
 * whole track is any rating, unrated books included.
 */
const RatingRangeSlider = ({
  labelId,
  min,
  max,
  onChange,
}: {
  labelId: string;
  min: number | undefined;
  max: number | undefined;
  onChange: (min: number | undefined, max: number | undefined) => void;
}) => {
  const committed: [number, number] = [
    min ?? RATING_FLOOR,
    max ?? RATING_CEILING,
  ];
  // The range shown while dragging, before it's committed to the URL.
  const [preview, setPreview] = useState<[number, number] | null>(null);
  // Radix reports a key press's commit before its change, so the preview
  // clears once the committed range arrives rather than on commit.
  useEffect(() => setPreview(null), [min, max]);
  const [low, high] = preview ?? committed;
  const range = (lo: number, hi: number) => ({
    min: lo === RATING_FLOOR ? undefined : lo,
    // The top of the range is at least the lowest rating there is.
    max: hi === RATING_CEILING ? undefined : Math.max(hi, 0.25),
  });
  const shown = range(low, high);

  return (
    <div className="space-y-2">
      <div className="text-right text-xs tabular-nums text-muted-foreground">
        {ratingRangeLabel(shown.min, shown.max)}
      </div>
      <SliderPrimitive.Root
        aria-labelledby={labelId}
        className="relative flex w-full touch-none select-none items-center py-2"
        max={RATING_CEILING}
        min={RATING_FLOOR}
        minStepsBetweenThumbs={0}
        onValueChange={([lo, hi]) => setPreview([lo, hi])}
        onValueCommit={([lo, hi]) => {
          const next = range(lo, hi);
          onChange(next.min, next.max);
        }}
        step={0.25}
        value={[low, high]}
      >
        <SliderPrimitive.Track className="relative h-1.5 w-full grow overflow-hidden rounded-full bg-amber-400/20">
          <SliderPrimitive.Range className="absolute h-full bg-amber-400" />
        </SliderPrimitive.Track>
        {(
          [
            ["Lowest rating", low, RATING_FLOOR],
            ["Highest rating", high, RATING_CEILING],
          ] as const
        ).map(([label, stars, edge]) => (
          <SliderPrimitive.Thumb
            aria-label={label}
            aria-valuetext={ratingEndText(stars, edge)}
            className="block h-4 w-4 rounded-full border border-amber-500 bg-background shadow transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            key={label}
          />
        ))}
      </SliderPrimitive.Root>
      <div
        aria-hidden
        className="flex justify-between text-[10px] text-muted-foreground"
      >
        <span>Any</span>
        <span>Any</span>
      </div>
    </div>
  );
};

/**
 * The Reading Status and rating filters. Both read the signed-in user's own
 * reading, so they're the same on a library and on a List.
 */
export const ReadingFilterSection = ({
  filters,
  onChange,
}: {
  filters: ReadingFilters;
  onChange: (next: ReadingFilters) => void;
}) => {
  const statusLabelId = useId();
  const ratingLabelId = useId();

  return (
    <>
      <div>
        <SectionHeader
          colorClass="bg-chart-1/20 text-chart-1"
          detail={
            filters.statuses.length > 0
              ? `${filters.statuses.length} selected`
              : undefined
          }
          icon={<BookOpenCheck className="h-3 w-3" />}
          label="Reading status"
          labelId={statusLabelId}
        />
        <MultiSelectCombobox
          labelId={statusLabelId}
          onToggle={(value) =>
            onChange({
              ...filters,
              statuses: filters.statuses.includes(value)
                ? filters.statuses.filter((s) => s !== value)
                : [...filters.statuses, value],
            })
          }
          options={READING_STATUS_FILTER_OPTIONS}
          placeholder="Any status"
          selected={filters.statuses}
        />
      </div>

      <div>
        <SectionHeader
          colorClass="bg-amber-400/20 text-amber-500"
          icon={<Star className="h-3 w-3" />}
          label="Rating"
          labelId={ratingLabelId}
        />
        <RatingRangeSlider
          labelId={ratingLabelId}
          max={filters.ratingMax}
          min={filters.ratingMin}
          onChange={(min, max) => {
            const next: ReadingFilters = { statuses: filters.statuses };
            if (min !== undefined) next.ratingMin = min;
            if (max !== undefined) next.ratingMax = max;
            onChange(next);
          }}
        />
      </div>
    </>
  );
};

const scopeKey = (scope: TagScope) =>
  scope.kind === "namespace" ? `ns:${scope.namespace}` : scope.kind;

// Narrows the tag picker to every tag, the plain tags, or one namespace.
const TagScopeSwitch = ({
  namespaces,
  scope,
  onChange,
}: {
  namespaces: readonly string[];
  scope: TagScope;
  onChange: (scope: TagScope) => void;
}) => {
  const scopes: { scope: TagScope; label: string }[] = [
    { scope: { kind: "all" }, label: "All" },
    { scope: { kind: "plain" }, label: "Tags" },
    ...namespaces.map((namespace) => ({
      scope: { kind: "namespace", namespace } as TagScope,
      label: namespace,
    })),
  ];
  return (
    <div
      aria-label="Show tags from"
      className="flex flex-wrap gap-1 border-b border-border p-2"
      role="group"
    >
      {scopes.map((s) => {
        const active = scopeKey(s.scope) === scopeKey(scope);
        return (
          <Button
            aria-pressed={active}
            className={cn(
              "inline-flex h-7 items-center rounded-md border px-2.5 text-xs font-medium transition-colors",
              active
                ? "border-primary bg-primary/5 text-primary"
                : "border-border bg-card hover:bg-accent",
            )}
            key={scopeKey(s.scope)}
            onClick={(e) => {
              onChange(s.scope);
              // Back to the search box, so typing narrows the new scope and
              // Escape closes the picker rather than landing on this button.
              e.currentTarget
                .closest("[cmdk-root]")
                ?.querySelector<HTMLInputElement>("[cmdk-input]")
                ?.focus();
            }}
            variant="unstyled"
          >
            {s.label}
          </Button>
        );
      })}
    </div>
  );
};

const FilterContent = ({
  reading,
  onReadingChange,
  selectedFileTypes,
  fileTypeOptions,
  onToggleFileType,
  selectedGenreIds,
  selectedGenres,
  genres,
  genresLoading,
  genresQuery,
  genreSearchInput,
  onGenreSearchChange,
  onToggleGenre,
  selectedTagIds,
  selectedTags,
  tags,
  tagsLoading,
  tagsQuery,
  tagSearchInput,
  onTagSearchChange,
  onToggleTag,
  tagNamespaces,
  tagScope,
  onTagScopeChange,
  languageParam,
  languageOptions,
  onLanguageChange,
  reviewedFilter,
  onReviewedFilterChange,
}: Omit<FilterSheetProps, "onClearAll" | "hasActiveFilters">) => {
  const genresLabelId = useId();
  const tagsLabelId = useId();
  const languageLabelId = useId();
  const fileTypeLabelId = useId();
  const reviewLabelId = useId();
  return (
    <div className="space-y-6">
      <ReadingFilterSection filters={reading} onChange={onReadingChange} />

      {/* File Type */}
      <div>
        <SectionHeader
          colorClass="bg-chart-5/20 text-chart-5"
          detail={
            selectedFileTypes.length > 0
              ? `${selectedFileTypes.length} of ${fileTypeOptions.length}`
              : undefined
          }
          icon={<File className="h-3 w-3" />}
          label="File type"
          labelId={fileTypeLabelId}
        />
        <MultiSelectCombobox
          labelId={fileTypeLabelId}
          onToggle={onToggleFileType}
          options={fileTypeOptions}
          placeholder="Any file type"
          selected={selectedFileTypes}
        />
      </div>

      {/* Genres */}
      <div>
        <SectionHeader
          colorClass="bg-primary/20 text-primary"
          detail={
            selectedGenreIds.length > 0
              ? `${selectedGenreIds.length} selected`
              : undefined
          }
          icon={<Bookmark className="h-3 w-3" />}
          label="Genres"
          labelId={genresLabelId}
        />
        <FilterCombobox
          chipKind="genre"
          emptyText="No genres found."
          errorFallback="Failed to load genres"
          label="Genres"
          labelId={genresLabelId}
          loading={genresLoading}
          onSearchChange={onGenreSearchChange}
          onToggle={onToggleGenre}
          options={genres}
          query={genresQuery}
          searchInput={genreSearchInput}
          searchPlaceholder="Search genres..."
          selected={selectedGenres}
          triggerLabel="Add genres..."
        />
      </div>

      {/* Tags */}
      <div>
        <SectionHeader
          colorClass="bg-chart-2/20 text-chart-2"
          detail={
            selectedTagIds.length > 0
              ? `${selectedTagIds.length} selected`
              : undefined
          }
          icon={<Tags className="h-3 w-3" />}
          label="Tags"
          labelId={tagsLabelId}
        />
        <FilterCombobox
          chipKind="tag"
          emptyText="No tags found."
          errorFallback="Failed to load tags"
          header={
            tagNamespaces.length > 0 && (
              <TagScopeSwitch
                namespaces={tagNamespaces}
                onChange={onTagScopeChange}
                scope={tagScope}
              />
            )
          }
          label="Tags"
          labelId={tagsLabelId}
          loading={tagsLoading}
          onSearchChange={onTagSearchChange}
          onToggle={onToggleTag}
          optionLabel={(opt) => {
            const split = splitTagNamespace(opt.name, tagNamespaces);
            // Under All, a namespaced tag names its namespace.
            if (!split.namespace || tagScope.kind !== "all") return split.name;
            return (
              <>
                <span className="rounded bg-muted px-1 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                  {split.namespace}
                </span>
                {split.name}
              </>
            );
          }}
          options={tags}
          query={tagsQuery}
          searchInput={tagSearchInput}
          searchPlaceholder={
            tagScope.kind === "namespace"
              ? `Search ${tagScope.namespace}...`
              : "Search tags..."
          }
          selected={selectedTags}
          triggerLabel="Add tags..."
        />
      </div>

      {/* Language */}
      {languageOptions.length > 0 && (
        <div>
          <SectionHeader
            colorClass="bg-chart-5/20 text-chart-5"
            icon={<Languages className="h-3 w-3" />}
            label="Language"
            labelId={languageLabelId}
          />
          <Select
            onValueChange={onLanguageChange}
            value={languageParam || "all"}
          >
            <SelectTrigger aria-labelledby={languageLabelId} className="w-full">
              <SelectValue placeholder="Language" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Languages</SelectItem>
              {languageOptions.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {/* Review status */}
      <div>
        <SectionHeader
          colorClass="bg-chart-3/20 text-chart-3"
          icon={<Eye className="h-3 w-3" />}
          label="Review status"
          labelId={reviewLabelId}
        />
        <RadioGroup
          aria-labelledby={reviewLabelId}
          className="gap-2"
          onValueChange={onReviewedFilterChange}
          value={reviewedFilter || "all"}
        >
          <div className="flex items-center gap-2">
            <RadioGroupItem id="review-all" value="all" />
            <Label className="cursor-pointer font-normal" htmlFor="review-all">
              All
            </Label>
          </div>
          <div className="flex items-center gap-2">
            <RadioGroupItem id="review-needs" value="needs_review" />
            <Label
              className="cursor-pointer font-normal"
              htmlFor="review-needs"
            >
              Needs review
            </Label>
          </div>
          <div className="flex items-center gap-2">
            <RadioGroupItem id="review-reviewed" value="reviewed" />
            <Label
              className="cursor-pointer font-normal"
              htmlFor="review-reviewed"
            >
              Reviewed
            </Label>
          </div>
        </RadioGroup>
      </div>
    </div>
  );
};

// forwardRef so SheetTrigger/DrawerTrigger asChild can attach its DOM ref
// onto the underlying button. See the Radix `asChild` trigger gotcha in
// app/AGENTS.md.
const FilterButton = forwardRef<
  HTMLButtonElement,
  { hasActiveFilters: boolean } & Omit<
    React.ComponentPropsWithoutRef<typeof Button>,
    "hasActiveFilters"
  >
>(({ hasActiveFilters, ...props }, ref) => (
  <Button ref={ref} size="sm" variant="outline" {...props}>
    <ListFilter className="h-4 w-4" />
    Filter
    {hasActiveFilters && (
      <span
        aria-label="Filters are active"
        className="absolute top-1 right-1 h-2 w-2 rounded-full bg-primary ring-2 ring-background"
      />
    )}
  </Button>
));
FilterButton.displayName = "FilterButton";

interface FilterPanelProps {
  hasActiveFilters: boolean;
  onClearAll: () => void;
  /** Says what the filters filter by, for screen readers. */
  description: string;
  children: ReactNode;
}

/**
 * The Filter button and the sheet (a drawer on phones) it opens, holding
 * children as the filter sections.
 */
export const FilterPanel = ({
  hasActiveFilters,
  onClearAll,
  description,
  children,
}: FilterPanelProps) => {
  const isDesktop = useMediaQuery("(min-width: 768px)");

  if (isDesktop) {
    return (
      <Sheet>
        <SheetTrigger asChild>
          <FilterButton
            className="relative"
            hasActiveFilters={hasActiveFilters}
          />
        </SheetTrigger>
        <SheetContent className="flex flex-col overflow-hidden">
          <SheetHeader className="flex flex-row items-center justify-between space-y-0 pb-4">
            <SheetTitle>Filters</SheetTitle>
            <SheetDescription className="sr-only">
              {description}
            </SheetDescription>
            {hasActiveFilters && (
              <Button
                className="text-xs font-normal text-muted-foreground hover:text-foreground hover:no-underline"
                onClick={onClearAll}
                variant="link"
              >
                Clear all
              </Button>
            )}
          </SheetHeader>
          <div className="flex-1 overflow-y-auto pr-1">{children}</div>
          <SheetFooter className="pt-4">
            <SheetClose asChild>
              <Button className="w-full sm:w-auto">Done</Button>
            </SheetClose>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    );
  }

  return (
    <Drawer>
      <DrawerTrigger asChild>
        <FilterButton
          className="relative"
          hasActiveFilters={hasActiveFilters}
        />
      </DrawerTrigger>
      <DrawerContent>
        <DrawerHeader className="flex flex-row items-center justify-between">
          <DrawerTitle>Filters</DrawerTitle>
          <DrawerDescription className="sr-only">
            {description}
          </DrawerDescription>
          {hasActiveFilters && (
            <Button
              className="text-xs font-normal text-muted-foreground hover:text-foreground hover:no-underline"
              onClick={onClearAll}
              variant="link"
            >
              Clear all
            </Button>
          )}
        </DrawerHeader>
        <div className="overflow-y-auto px-4 pb-4 max-h-[70vh]">{children}</div>
        <DrawerFooter>
          <DrawerClose asChild>
            <Button>Done</Button>
          </DrawerClose>
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  );
};

export const FilterSheet = (props: FilterSheetProps) => {
  const { onClearAll, hasActiveFilters, ...filterContentProps } = props;
  return (
    <FilterPanel
      description="Filter books by reading status, rating, file type, genre, tag, or language"
      hasActiveFilters={hasActiveFilters}
      onClearAll={onClearAll}
    >
      <FilterContent {...filterContentProps} />
    </FilterPanel>
  );
};
