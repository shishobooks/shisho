import type { ReactNode } from "react";

import { cn } from "@/libraries/utils";

export interface LabelGridRow {
  label: string;
  value: ReactNode;
}

/**
 * Facts as a two-column grid of small uppercase labels and their values,
 * the way the book and file pages list series, publisher, identifiers and
 * the like. A long label (a plugin's identifier type) wraps in its column.
 */
const LabelGrid = ({
  rows,
  className,
}: {
  rows: LabelGridRow[];
  className?: string;
}) =>
  rows.length > 0 ? (
    <dl
      className={cn(
        "grid grid-cols-[6.5rem_minmax(0,1fr)] gap-x-4 gap-y-1.5 text-sm",
        className,
      )}
    >
      {rows.map((row) => (
        <div className="contents" key={row.label}>
          <dt className="break-words pt-0.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
            {row.label}
          </dt>
          <dd className="min-w-0 break-words">{row.value}</dd>
        </div>
      ))}
    </dl>
  ) : null;

export default LabelGrid;
