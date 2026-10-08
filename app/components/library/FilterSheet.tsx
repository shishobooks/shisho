import {
  Bookmark,
  ChevronsUpDown,
  Eye,
  File,
  Languages,
  ListFilter,
  Square,
  SquareCheckBig,
  Tags,
} from "lucide-react";
import { forwardRef, useId, useState } from "react";

import { FilterChip } from "@/components/library/FilterChip";
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
import { cn } from "@/libraries/utils";
import type { Genre, Tag } from "@/types";

type FilterOptionsQuery = RetryableQuery & { data: unknown };

interface FilterSheetProps {
  selectedFileTypes: string[];
  fileTypeOptions: readonly { value: string; label: string }[];
  onToggleFileType: (fileType: string) => void;
  selectedGenreIds: number[];
  selectedGenres: Genre[];
  genres: Genre[];
  genresLoading: boolean;
  /** The genres query, for its error report and Retry. */
  genresQuery: FilterOptionsQuery;
  genreSearchInput: string;
  onGenreSearchChange: (value: string) => void;
  onToggleGenre: (genreId: number) => void;
  selectedTagIds: number[];
  selectedTags: Tag[];
  tags: Tag[];
  tagsLoading: boolean;
  /** The tags query, for its error report and Retry. */
  tagsQuery: FilterOptionsQuery;
  tagSearchInput: string;
  onTagSearchChange: (value: string) => void;
  onToggleTag: (tagId: number) => void;
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
}: {
  chipKind: "genre" | "tag";
  selected: FilterComboboxOption[];
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
                      {opt.name}
                      <span className="ml-auto text-xs text-muted-foreground">
                        {opt.book_count}
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

const FilterContent = ({
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
  languageParam,
  languageOptions,
  onLanguageChange,
  reviewedFilter,
  onReviewedFilterChange,
}: Omit<FilterSheetProps, "onClearAll" | "hasActiveFilters">) => {
  const genresLabelId = useId();
  const tagsLabelId = useId();
  const languageLabelId = useId();
  return (
    <div className="space-y-6">
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
        />
        <div className="flex flex-wrap gap-1.5">
          {fileTypeOptions.map((option) => {
            const isSelected = selectedFileTypes.includes(option.value);
            return (
              <Button
                aria-pressed={isSelected}
                className={cn(
                  "inline-flex h-8 items-center rounded-md border px-3 text-xs font-medium transition-colors",
                  isSelected
                    ? "border-primary bg-primary/5 text-primary"
                    : "border-border bg-card hover:bg-accent",
                )}
                key={option.value}
                onClick={() => onToggleFileType(option.value)}
                variant="unstyled"
              >
                {option.label}
              </Button>
            );
          })}
        </div>
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
          label="Tags"
          labelId={tagsLabelId}
          loading={tagsLoading}
          onSearchChange={onTagSearchChange}
          onToggle={onToggleTag}
          options={tags}
          query={tagsQuery}
          searchInput={tagSearchInput}
          searchPlaceholder="Search tags..."
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

      {/* Review state */}
      <div>
        <SectionHeader
          colorClass="bg-chart-3/20 text-chart-3"
          icon={<Eye className="h-3 w-3" />}
          label="Review state"
        />
        <RadioGroup
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

export const FilterSheet = (props: FilterSheetProps) => {
  const { onClearAll, hasActiveFilters, ...filterContentProps } = props;
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
              Filter books by file type, genre, tag, or language
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
          <div className="flex-1 overflow-y-auto pr-1">
            <FilterContent {...filterContentProps} />
          </div>
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
            Filter books by file type, genre, tag, or language
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
        <div className="overflow-y-auto px-4 pb-4 max-h-[70vh]">
          <FilterContent {...filterContentProps} />
        </div>
        <DrawerFooter>
          <DrawerClose asChild>
            <Button>Done</Button>
          </DrawerClose>
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  );
};
