import {
  FilterChip,
  type SelectedFilter,
} from "@/components/library/FilterChip";
import { Button } from "@/components/ui/button";
import { FILE_TYPE_OPTIONS } from "@/constants/fileTypes";
import { getLanguageName } from "@/constants/languages";
import {
  ratingRangeLabel,
  readingStatusFilterLabel,
  type ReadingFilters,
} from "@/libraries/readingFilters";

const REVIEWED_FILTER_LABELS: Record<string, string> = {
  needs_review: "Needs review",
  reviewed: "Reviewed",
};

// The library's own filters are optional, since a List has only the reading
// filters.
interface ActiveFilterChipsProps {
  hasActiveFilters: boolean;
  reading: ReadingFilters;
  onReadingChange: (next: ReadingFilters) => void;
  selectedFileTypes?: string[];
  selectedGenres?: SelectedFilter[];
  selectedTags?: SelectedFilter[];
  languageParam?: string;
  reviewedFilter?: string;
  onToggleFileType?: (fileType: string) => void;
  onToggleGenre?: (genreId: number) => void;
  onToggleTag?: (tagId: number) => void;
  onClearLanguage?: () => void;
  onClearReviewedFilter?: () => void;
  onClearAll: () => void;
}

export const ActiveFilterChips = ({
  hasActiveFilters,
  reading,
  onReadingChange,
  selectedFileTypes = [],
  selectedGenres = [],
  selectedTags = [],
  languageParam = "",
  reviewedFilter = "",
  onToggleFileType,
  onToggleGenre,
  onToggleTag,
  onClearLanguage,
  onClearReviewedFilter,
  onClearAll,
}: ActiveFilterChipsProps) => {
  if (!hasActiveFilters) return null;

  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-xs text-muted-foreground">Filtering by</span>
      {reading.statuses.map((status) => (
        <FilterChip
          key={status}
          kind="readingStatus"
          label={readingStatusFilterLabel(status)}
          onRemove={() =>
            onReadingChange({
              ...reading,
              statuses: reading.statuses.filter((s) => s !== status),
            })
          }
        />
      ))}
      {(reading.ratingMin !== undefined || reading.ratingMax !== undefined) && (
        <FilterChip
          kind="rating"
          label={ratingRangeLabel(reading.ratingMin, reading.ratingMax)}
          onRemove={() => onReadingChange({ statuses: reading.statuses })}
        />
      )}
      {selectedFileTypes.map((fileType) => (
        <FilterChip
          key={fileType}
          kind="fileType"
          label={
            FILE_TYPE_OPTIONS.find((o) => o.value === fileType)?.label ??
            fileType
          }
          onRemove={() => onToggleFileType?.(fileType)}
        />
      ))}
      {selectedGenres.map((genre) => (
        <FilterChip
          key={genre.id}
          kind="genre"
          label={genre.name}
          onRemove={() => onToggleGenre?.(genre.id)}
        />
      ))}
      {selectedTags.map((tag) => (
        <FilterChip
          key={tag.id}
          kind="tag"
          label={tag.name}
          onRemove={() => onToggleTag?.(tag.id)}
        />
      ))}
      {languageParam && (
        <FilterChip
          kind="language"
          label={getLanguageName(languageParam) ?? languageParam}
          onRemove={() => onClearLanguage?.()}
        />
      )}
      {reviewedFilter && reviewedFilter !== "all" && (
        <FilterChip
          kind="reviewStatus"
          label={REVIEWED_FILTER_LABELS[reviewedFilter] ?? reviewedFilter}
          onRemove={() => onClearReviewedFilter?.()}
        />
      )}
      <Button
        className="ml-1 text-xs font-normal text-muted-foreground underline-offset-2 hover:text-foreground"
        onClick={onClearAll}
        variant="link"
      >
        clear all
      </Button>
    </div>
  );
};
