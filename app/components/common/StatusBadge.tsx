import { Badge } from "@/components/ui/badge";
import { cn } from "@/libraries/utils";

export type EntityStatus = "new" | "changed" | "unchanged";

interface StatusBadgeProps {
  status: EntityStatus;
  /** Render the smaller per-row variant (used inside chips/list rows). */
  size?: "default" | "sm";
}

/**
 * Subtle status badge used in the identify review form to mark whether a
 * field/row is new, changed, or unchanged from the current value. Color
 * tokens are semantic (emerald for new, primary for changed) and dark-mode
 * aware so all the per-field and per-row badges share one visual language.
 */
export function StatusBadge({ status, size = "default" }: StatusBadgeProps) {
  const sizeClass =
    size === "sm" ? "text-[0.6rem] px-1.5 py-0" : "text-[0.65rem]";

  if (status === "new") {
    return (
      <Badge
        className={cn(
          sizeClass,
          "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-400 border-transparent",
        )}
        data-testid="entity-status-badge"
        variant="outline"
      >
        New
      </Badge>
    );
  }
  if (status === "changed") {
    return (
      <Badge
        className={cn(
          sizeClass,
          "bg-primary/10 text-primary dark:bg-primary/20 border-transparent",
        )}
        data-testid="entity-status-badge"
        variant="outline"
      >
        Changed
      </Badge>
    );
  }
  return (
    <Badge
      className={sizeClass}
      data-testid="entity-status-badge"
      variant="outline"
    >
      Unchanged
    </Badge>
  );
}
