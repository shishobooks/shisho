import { FilterChip } from "@/components/library/FilterChip";
import { FILE_TYPE_OPTIONS } from "@/constants/fileTypes";
import { getLanguageName } from "@/constants/languages";
import type { Genre, Tag } from "@/types";

const REVIEWED_FILTER_LABELS: Record<string, string> = {
  needs_review: "Needs review",
  reviewed: "Reviewed",
};

interface ActiveFilterChipsProps {
  hasActiveFilters: boolean;
  selectedFileTypes: string[];
  selectedGenres: Genre[];
  selectedTags: Tag[];
  languageParam: string;
  reviewedFilter: string;
  onToggleFileType: (fileType: string) => void;
  onToggleGenre: (genreId: number) => void;
  onToggleTag: (tagId: number) => void;
  onClearLanguage: () => void;
  onClearReviewedFilter: () => void;
  onClearAll: () => void;
}

export const ActiveFilterChips = ({
  hasActiveFilters,
  selectedFileTypes,
  selectedGenres,
  selectedTags,
  languageParam,
  reviewedFilter,
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
      {selectedFileTypes.map((fileType) => (
        <FilterChip
          key={fileType}
          kind="fileType"
          label={
            FILE_TYPE_OPTIONS.find((o) => o.value === fileType)?.label ??
            fileType
          }
          onRemove={() => onToggleFileType(fileType)}
        />
      ))}
      {selectedGenres.map((genre) => (
        <FilterChip
          key={genre.id}
          kind="genre"
          label={genre.name}
          onRemove={() => onToggleGenre(genre.id)}
        />
      ))}
      {selectedTags.map((tag) => (
        <FilterChip
          key={tag.id}
          kind="tag"
          label={tag.name}
          onRemove={() => onToggleTag(tag.id)}
        />
      ))}
      {languageParam && (
        <FilterChip
          kind="language"
          label={getLanguageName(languageParam) ?? languageParam}
          onRemove={onClearLanguage}
        />
      )}
      {reviewedFilter && reviewedFilter !== "all" && (
        <FilterChip
          kind="reviewState"
          label={REVIEWED_FILTER_LABELS[reviewedFilter] ?? reviewedFilter}
          onRemove={onClearReviewedFilter}
        />
      )}
      <button
        className="text-xs text-muted-foreground underline-offset-2 hover:underline hover:text-foreground ml-1 cursor-pointer"
        onClick={onClearAll}
      >
        clear all
      </button>
    </div>
  );
};
