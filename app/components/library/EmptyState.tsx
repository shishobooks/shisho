import { SearchX, type LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/libraries/utils";

interface EmptyStateProps {
  /** The icon the nav shows for this place, so the two match. */
  icon: LucideIcon;
  /** Short, names the state: "No devices yet". */
  title: string;
  /**
   * What this place is for and how it fills up, in a sentence or two, never
   * just "No X found".
   */
  description: ReactNode;
  /**
   * The next step, when one exists and the role may take it: a `Button`, or
   * a `Button asChild` around a `Link`. Callers gate it with `useCan`. An
   * action that creates the thing this place holds is a default `Button`
   * with a `Plus` icon; one that goes elsewhere is `variant="outline"`.
   */
  action?: ReactNode;
  /**
   * Heading level, one below the heading of the section it sits in, or 1
   * when the empty state is the whole page.
   */
  headingLevel?: 1 | 2 | 3;
  /** Drops the outline for a spot already inside a bordered box. */
  bare?: boolean;
  className?: string;
}

/**
 * What an empty page or list shows in place of its items. It is for a page's
 * main content and for lists a page is built around; a small list inside a
 * dialog, popover, combobox, settings editor, or chart card keeps a one-line
 * note, since the icon and spacing would crowd the control.
 */
const EmptyState = ({
  icon: Icon,
  title,
  description,
  action,
  headingLevel = 2,
  bare = false,
  className,
}: EmptyStateProps) => {
  const Heading = `h${headingLevel}` as const;
  return (
    <div
      className={cn(
        "flex flex-col items-center px-4 py-10 text-center md:py-12",
        !bare && "rounded-md border border-dashed border-border",
        className,
      )}
    >
      <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 text-primary">
        <Icon aria-hidden="true" className="h-6 w-6" />
      </div>
      <Heading className="mt-4 text-base font-semibold text-foreground">
        {title}
      </Heading>
      <p className="mt-1 max-w-md text-sm text-muted-foreground">
        {description}
      </p>
      {action && (
        <div className="mt-5 flex flex-wrap justify-center gap-2">{action}</div>
      )}
    </div>
  );
};

interface NoMatchesStateProps {
  /** The plural noun for the items: "books". */
  itemLabel: string;
  /** Overrides the default hint, e.g. when the list has filters but no search. */
  description?: ReactNode;
  action?: ReactNode;
  headingLevel?: 1 | 2 | 3;
  bare?: boolean;
}

/** A list whose search or filters match nothing, as opposed to an empty one. */
export const NoMatchesState = ({
  itemLabel,
  description = "Try a different search or fewer filters.",
  ...props
}: NoMatchesStateProps) => (
  <EmptyState
    description={description}
    icon={SearchX}
    title={`No matching ${itemLabel}`}
    {...props}
  />
);

export default EmptyState;
