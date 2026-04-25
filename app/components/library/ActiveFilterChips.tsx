import { Bookmark, Eye, File, Languages, Tags, X } from "lucide-react";
import type { ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import { FILE_TYPE_OPTIONS } from "@/constants/fileTypes";
import { getLanguageName } from "@/constants/languages";
import type { Genre, Tag } from "@/types";

interface FilterTypeConfig {
  icon: ReactNode;
  colorClass: string;
}

const FILTER_TYPES: Record<
  "fileType" | "genre" | "tag" | "language" | "reviewState",
  FilterTypeConfig
> = {
  fileType: {
    icon: <File className="h-3 w-3 shrink-0" />,
    colorClass: "text-chart-5",
  },
  genre: {
    icon: <Bookmark className="h-3 w-3 shrink-0" />,
    colorClass: "text-primary",
  },
  tag: {
    icon: <Tags className="h-3 w-3 shrink-0" />,
    colorClass: "text-chart-2",
  },
  language: {
    icon: <Languages className="h-3 w-3 shrink-0" />,
    colorClass: "text-chart-5",
  },
  reviewState: {
    icon: <Eye className="h-3 w-3 shrink-0" />,
    colorClass: "text-chart-3",
  },
};

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
        <Badge
          className="cursor-pointer gap-1.5"
          key={fileType}
          onClick={() => onToggleFileType(fileType)}
          variant="secondary"
        >
          <span className={FILTER_TYPES.fileType.colorClass}>
            {FILTER_TYPES.fileType.icon}
          </span>
          {FILE_TYPE_OPTIONS.find((o) => o.value === fileType)?.label ??
            fileType}
          <X className="h-3 w-3 text-muted-foreground" />
        </Badge>
      ))}
      {selectedGenres.map((genre) => (
        <Badge
          className="cursor-pointer gap-1.5"
          key={genre.id}
          onClick={() => onToggleGenre(genre.id)}
          variant="secondary"
        >
          <span className={FILTER_TYPES.genre.colorClass}>
            {FILTER_TYPES.genre.icon}
          </span>
          {genre.name}
          <X className="h-3 w-3 text-muted-foreground" />
        </Badge>
      ))}
      {selectedTags.map((tag) => (
        <Badge
          className="cursor-pointer gap-1.5"
          key={tag.id}
          onClick={() => onToggleTag(tag.id)}
          variant="secondary"
        >
          <span className={FILTER_TYPES.tag.colorClass}>
            {FILTER_TYPES.tag.icon}
          </span>
          {tag.name}
          <X className="h-3 w-3 text-muted-foreground" />
        </Badge>
      ))}
      {languageParam && (
        <Badge
          className="cursor-pointer gap-1.5"
          onClick={onClearLanguage}
          variant="secondary"
        >
          <span className={FILTER_TYPES.language.colorClass}>
            {FILTER_TYPES.language.icon}
          </span>
          {getLanguageName(languageParam) ?? languageParam}
          <X className="h-3 w-3 text-muted-foreground" />
        </Badge>
      )}
      {reviewedFilter && reviewedFilter !== "all" && (
        <Badge
          className="cursor-pointer gap-1.5"
          onClick={onClearReviewedFilter}
          variant="secondary"
        >
          <span className={FILTER_TYPES.reviewState.colorClass}>
            {FILTER_TYPES.reviewState.icon}
          </span>
          {REVIEWED_FILTER_LABELS[reviewedFilter] ?? reviewedFilter}
          <X className="h-3 w-3 text-muted-foreground" />
        </Badge>
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
