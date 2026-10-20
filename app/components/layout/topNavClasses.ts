import { cn } from "@/libraries/utils";

// The outer top-nav geometry, shared by the library TopNav and AdminHeader so
// the two stay identical. The row height plus the wrapper's 1px border is the
// offset Sidebar and other full-height surfaces subtract below the nav; change
// them together.

export const TOP_NAV_WRAPPER = cn(
  "sticky top-[var(--demo-banner-height,0px)] z-30 border-b border-border bg-background",
);
// The nav spans the full window rather than a centered column: pages with a
// sidebar center their body to the right of it, so a centered nav column
// would sit offset from the body by half the sidebar's width.
export const TOP_NAV_INNER = cn("px-4 md:px-6");
export const TOP_NAV_ROW = cn("flex h-14 items-center justify-between md:h-16");
