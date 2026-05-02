import { ChevronDown } from "lucide-react";
import type * as React from "react";

import { Checkbox } from "@/components/ui/checkbox";
import { cn } from "@/libraries/utils";

interface Props {
  label: string;
  hint?: React.ReactNode;
  selectedCount: number;
  totalCount: number;
  collapsed: boolean;
  checkboxState: boolean | "indeterminate";
  onToggleCollapse: () => void;
  onCheckedChange: (checked: boolean) => void;
  className?: string;
}

/** Sticky banner heading a Book / File section in the identify dialog.
 *
 * Click the banner body (anywhere except the checkbox) to toggle collapse.
 * The checkbox stops propagation so it doesn't trigger collapse. */
export function IdentifySectionBanner({
  label,
  hint,
  selectedCount,
  totalCount,
  collapsed,
  checkboxState,
  onToggleCollapse,
  onCheckedChange,
  className,
}: Props) {
  return (
    <div
      className={cn(
        "sticky z-[2] flex items-center gap-3 border-b bg-muted/40 px-5 py-2.5",
        className,
      )}
    >
      <button
        aria-controls={`identify-section-${label.toLowerCase()}`}
        aria-expanded={!collapsed}
        aria-label={`Toggle ${label} section`}
        className="-m-1 flex flex-1 cursor-pointer items-center gap-3 rounded p-1 text-left"
        onClick={onToggleCollapse}
        type="button"
      >
        <ChevronDown
          aria-hidden
          className={cn(
            "size-3 shrink-0 text-muted-foreground transition-transform",
            collapsed && "-rotate-90",
          )}
        />
        <span className="text-[11px] font-bold uppercase tracking-[0.14em] text-foreground/90">
          {label}
        </span>
        {hint != null && (
          <span className="min-w-0 truncate text-[11.5px] text-muted-foreground">
            {hint}
          </span>
        )}
        <span className="ml-auto whitespace-nowrap text-[11.5px] tabular-nums text-muted-foreground">
          <span className="font-semibold text-foreground">{selectedCount}</span>{" "}
          of {totalCount} selected
        </span>
      </button>
      <Checkbox
        aria-label={`Apply all ${label.toLowerCase()} fields`}
        checked={checkboxState}
        onCheckedChange={(v) => onCheckedChange(v === true)}
        onClick={(e) => e.stopPropagation()}
      />
    </div>
  );
}
