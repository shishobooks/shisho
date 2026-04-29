import { Bookmark, Eye, File, Languages, Tags, X } from "lucide-react";
import type { ReactNode } from "react";

import { Badge } from "@/components/ui/badge";

export type FilterChipKind =
  | "fileType"
  | "genre"
  | "tag"
  | "language"
  | "reviewState";

const KIND_CONFIG: Record<FilterChipKind, { icon: ReactNode; color: string }> =
  {
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
    <Badge
      asChild
      className="cursor-pointer gap-1.5 max-w-full"
      variant="secondary"
    >
      <button aria-label={`Remove ${label}`} onClick={onRemove} type="button">
        <span className={color}>{icon}</span>
        <span className="truncate" title={label}>
          {label}
        </span>
        <X className="h-3 w-3 text-muted-foreground shrink-0" />
      </button>
    </Badge>
  );
};
