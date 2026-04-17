import {
  Bookmark,
  File,
  Languages,
  ListFilter,
  Square,
  SquareCheckBig,
  Tags,
} from "lucide-react";

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
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@/components/ui/drawer";
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
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import type { Genre, Tag } from "@/types";

interface FilterSheetProps {
  selectedFileTypes: string[];
  fileTypeOptions: { value: string; label: string }[];
  onToggleFileType: (fileType: string) => void;
  selectedGenreIds: number[];
  genres: Genre[];
  genresLoading: boolean;
  genresError: boolean;
  genreSearchInput: string;
  onGenreSearchChange: (value: string) => void;
  onToggleGenre: (genreId: number) => void;
  selectedTagIds: number[];
  tags: Tag[];
  tagsLoading: boolean;
  tagsError: boolean;
  tagSearchInput: string;
  onTagSearchChange: (value: string) => void;
  onToggleTag: (tagId: number) => void;
  languageParam: string;
  languageOptions: { value: string; label: string }[];
  onLanguageChange: (value: string) => void;
  onClearAll: () => void;
  hasActiveFilters: boolean;
}

const SectionHeader = ({
  icon,
  colorClass,
  label,
  detail,
}: {
  icon: React.ReactNode;
  colorClass: string;
  label: string;
  detail?: string;
}) => (
  <div className="flex items-center gap-2 mb-3">
    <span
      className={`inline-flex h-5 w-5 items-center justify-center rounded-sm ${colorClass}`}
    >
      {icon}
    </span>
    <div className="text-xs font-semibold">{label}</div>
    {detail && (
      <span className="ml-auto text-[10px] text-muted-foreground">
        {detail}
      </span>
    )}
  </div>
);

const FilterContent = ({
  selectedFileTypes,
  fileTypeOptions,
  onToggleFileType,
  selectedGenreIds,
  genres,
  genresLoading,
  genresError,
  genreSearchInput,
  onGenreSearchChange,
  onToggleGenre,
  selectedTagIds,
  tags,
  tagsLoading,
  tagsError,
  tagSearchInput,
  onTagSearchChange,
  onToggleTag,
  languageParam,
  languageOptions,
  onLanguageChange,
}: Omit<FilterSheetProps, "onClearAll" | "hasActiveFilters">) => (
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
            <button
              className={`inline-flex h-8 items-center rounded-md border px-3 text-xs font-medium cursor-pointer transition-colors ${
                isSelected
                  ? "border-primary bg-primary text-primary-foreground"
                  : "border-border bg-card hover:bg-accent"
              }`}
              key={option.value}
              onClick={() => onToggleFileType(option.value)}
            >
              {option.label}
            </button>
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
      />
      <Command shouldFilter={false}>
        <CommandInput
          onValueChange={onGenreSearchChange}
          placeholder="Search genres..."
          value={genreSearchInput}
        />
        <CommandList>
          {genresLoading ? (
            <div className="py-6 text-center text-sm text-muted-foreground">
              Loading...
            </div>
          ) : genresError ? (
            <div className="py-6 text-center text-sm text-destructive">
              Error loading genres
            </div>
          ) : genres.length === 0 ? (
            <CommandEmpty>No genres found.</CommandEmpty>
          ) : (
            <CommandGroup>
              {genres.map((genre: Genre) => (
                <CommandItem
                  key={genre.id}
                  onSelect={() => onToggleGenre(genre.id)}
                  value={genre.name}
                >
                  {selectedGenreIds.includes(genre.id) ? (
                    <SquareCheckBig className="mr-2 h-4 w-4" />
                  ) : (
                    <Square className="mr-2 h-4 w-4" />
                  )}
                  {genre.name}
                  <span className="ml-auto text-xs text-muted-foreground">
                    {genre.book_count}
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
          )}
        </CommandList>
      </Command>
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
      />
      <Command shouldFilter={false}>
        <CommandInput
          onValueChange={onTagSearchChange}
          placeholder="Search tags..."
          value={tagSearchInput}
        />
        <CommandList>
          {tagsLoading ? (
            <div className="py-6 text-center text-sm text-muted-foreground">
              Loading...
            </div>
          ) : tagsError ? (
            <div className="py-6 text-center text-sm text-destructive">
              Error loading tags
            </div>
          ) : tags.length === 0 ? (
            <CommandEmpty>No tags found.</CommandEmpty>
          ) : (
            <CommandGroup>
              {tags.map((tag: Tag) => (
                <CommandItem
                  key={tag.id}
                  onSelect={() => onToggleTag(tag.id)}
                  value={tag.name}
                >
                  {selectedTagIds.includes(tag.id) ? (
                    <SquareCheckBig className="mr-2 h-4 w-4" />
                  ) : (
                    <Square className="mr-2 h-4 w-4" />
                  )}
                  {tag.name}
                  <span className="ml-auto text-xs text-muted-foreground">
                    {tag.book_count}
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
          )}
        </CommandList>
      </Command>
    </div>

    {/* Language */}
    {languageOptions.length > 0 && (
      <div>
        <SectionHeader
          colorClass="bg-chart-5/20 text-chart-5"
          icon={<Languages className="h-3 w-3" />}
          label="Language"
        />
        <Select onValueChange={onLanguageChange} value={languageParam || "all"}>
          <SelectTrigger className="w-full">
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
  </div>
);

const FilterButton = ({
  hasActiveFilters,
  ...props
}: {
  hasActiveFilters: boolean;
} & React.ButtonHTMLAttributes<HTMLButtonElement>) => (
  <Button aria-label="Filters" size="icon" variant="outline" {...props}>
    <ListFilter
      className={`h-4 w-4 ${hasActiveFilters ? "text-primary" : ""}`}
    />
    {hasActiveFilters && (
      <span
        className="absolute top-1 right-1 h-2 w-2 rounded-full bg-primary"
        style={{ boxShadow: "0 0 0 2px hsl(var(--background))" }}
      />
    )}
  </Button>
);

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
            {hasActiveFilters && (
              <button
                className="text-xs text-muted-foreground hover:text-foreground cursor-pointer"
                onClick={onClearAll}
              >
                Clear all
              </button>
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
          {hasActiveFilters && (
            <button
              className="text-xs text-muted-foreground hover:text-foreground cursor-pointer"
              onClick={onClearAll}
            >
              Clear all
            </button>
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
