import { ArrowDown, ArrowUp, X } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { SORT_FIELD_LABELS, type SortLevel } from "@/libraries/sortSpec";
import { cn } from "@/libraries/utils";

interface SortedByChipsProps {
  levels: readonly SortLevel[];
  onRemoveLevel: (index: number) => void;
  onReset: () => void;
}

const SortedByChips = ({
  levels,
  onRemoveLevel,
  onReset,
}: SortedByChipsProps) => {
  if (levels.length === 0) {
    return null;
  }

  return (
    <div className="flex flex-wrap items-center gap-2 py-2">
      <span className="text-sm text-muted-foreground">Sorted by:</span>

      {levels.map((level, index) => {
        const label = SORT_FIELD_LABELS[level.field];
        const Arrow = level.direction === "asc" ? ArrowUp : ArrowDown;
        const directionLabel =
          level.direction === "asc" ? "ascending" : "descending";
        return (
          <button
            aria-label={`${label} ${directionLabel} — click to remove`}
            className={cn("group cursor-pointer")}
            key={level.field}
            onClick={() => onRemoveLevel(index)}
            type="button"
          >
            <Badge
              className="gap-1 cursor-pointer hover:bg-destructive/20 transition-colors"
              variant="secondary"
            >
              <span>{label}</span>
              <Arrow className="h-3 w-3" />
              <X className="h-3 w-3 opacity-60 group-hover:opacity-100" />
            </Badge>
          </button>
        );
      })}

      <button
        className={cn(
          "text-sm text-muted-foreground underline underline-offset-2 hover:text-foreground cursor-pointer",
        )}
        onClick={onReset}
        type="button"
      >
        reset to default
      </button>
    </div>
  );
};

export default SortedByChips;
