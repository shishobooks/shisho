import { ArrowDown, ArrowUp, X } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { SORT_FIELD_LABELS, type SortLevel } from "@/libraries/sortSpec";

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
          <Button
            aria-label={`${label} ${directionLabel} — click to remove`}
            // field is unique per sort spec — safe to use as React key
            className="group h-auto p-0 hover:bg-transparent"
            key={level.field}
            onClick={() => onRemoveLevel(index)}
            size="sm"
            type="button"
            variant="ghost"
          >
            <Badge
              className="gap-1 hover:bg-destructive/20 transition-colors"
              variant="secondary"
            >
              <span>{label}</span>
              <Arrow className="h-3 w-3" />
              <X className="h-3 w-3 opacity-60 group-hover:opacity-100" />
            </Badge>
          </Button>
        );
      })}

      <Button
        className="h-auto p-0 text-sm text-muted-foreground hover:text-foreground"
        onClick={onReset}
        size="sm"
        type="button"
        variant="link"
      >
        reset to default
      </Button>
    </div>
  );
};

export default SortedByChips;
