import {
  Bookmark,
  BookOpenCheck,
  Eye,
  File,
  Languages,
  Star,
  Tags,
  X,
} from "lucide-react";
import type { ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

/** A selected genre or tag, as its chip shows it. */
export interface SelectedFilter {
  id: number;
  name: string;
}

export type FilterChipKind =
  | "readingStatus"
  | "rating"
  | "fileType"
  | "genre"
  | "tag"
  | "language"
  | "reviewState";

const KIND_CONFIG: Record<FilterChipKind, { icon: ReactNode; color: string }> =
  {
    readingStatus: {
      icon: <BookOpenCheck className="h-3 w-3 shrink-0" />,
      color: "text-chart-1",
    },
    rating: {
      icon: <Star className="h-3 w-3 shrink-0" />,
      color: "text-chart-4",
    },
    fileType: {
      icon: <File className="h-3 w-3 shrink-0" />,
      color: "text-chart-5",
    },
    genre: {
      icon: <Bookmark className="h-3 w-3 shrink-0" />,
      color: "text-primary",
    },
    tag: {
      icon: <Tags className="h-3 w-3 shrink-0" />,
      color: "text-chart-2",
    },
    language: {
      icon: <Languages className="h-3 w-3 shrink-0" />,
      color: "text-chart-5",
    },
    reviewState: {
      icon: <Eye className="h-3 w-3 shrink-0" />,
      color: "text-chart-3",
    },
  };

interface FilterChipProps {
  kind: FilterChipKind;
  label: string;
  onRemove: () => void;
}

export const FilterChip = ({ kind, label, onRemove }: FilterChipProps) => {
  const { icon, color } = KIND_CONFIG[kind];
  return (
    <Badge asChild className="gap-1.5 max-w-full" variant="secondary">
      <Button
        aria-label={`Remove ${label}`}
        onClick={onRemove}
        variant="unstyled"
      >
        <span className={color}>{icon}</span>
        <span className="truncate" title={label}>
          {label}
        </span>
        <X className="h-3 w-3 text-muted-foreground shrink-0" />
      </Button>
    </Badge>
  );
};
