import { useRef } from "react";

import type { PageChangeHandler } from "@/components/library/PaginationFooter";

// Room above the section once scrolled to, clear of the Demo Mode banner
// and the sticky top nav: its row (TOP_NAV_ROW) plus its 1px border.
export const SECTION_CLASS =
  "mb-10 scroll-mt-[calc(var(--demo-banner-height,0px)+57px+1rem)] md:scroll-mt-[calc(var(--demo-banner-height,0px)+65px+1rem)]";

/**
 * Paging for a section of a page, such as a person's narrated files below
 * their books. A page change is a navigation, and the app's root scrolls to
 * the top on navigation; for a section that would leave the reader far
 * above the page they asked for. So the change keeps the scroll, and the
 * section brings its own top into view instead, when it is above the
 * screen or under the sticky nav. Put `sectionRef` on the section, with
 * SECTION_CLASS, whose scroll margin is the room the nav takes.
 */
export const useSectionPaging = <T extends HTMLElement>(
  onPageChange: PageChangeHandler,
) => {
  const ref = useRef<T>(null);
  const sectionPageChange: PageChangeHandler = (page, options) => {
    onPageChange(page, { ...options, preventScrollReset: true });
    // After the new page renders, so the section has its new height.
    requestAnimationFrame(() => {
      const section = ref.current;
      if (!section) return;
      const covered = parseFloat(getComputedStyle(section).scrollMarginTop);
      if (section.getBoundingClientRect().top < (covered || 0)) {
        section.scrollIntoView({ block: "start" });
      }
    });
  };
  return { sectionRef: ref, sectionPageChange };
};
