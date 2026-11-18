import { useLayoutEffect, useRef } from "react";

import DemoBanner from "@/components/layout/DemoBanner";
import { NewBuildBanner } from "@/components/layout/NewBuild";

/**
 * Stacks the app-wide banners above every page and publishes their combined
 * height as --banner-height. Their height is not fixed (notices wrap on
 * phones, and each banner comes and goes), so full-height surfaces read the
 * variable, never a constant.
 */
const TopBanners = () => {
  const ref = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    const banners = ref.current;
    if (!banners) return;

    const updateHeight = () => {
      document.documentElement.style.setProperty(
        "--banner-height",
        `${banners.offsetHeight}px`,
      );
    };
    updateHeight();

    const observer = new ResizeObserver(updateHeight);
    observer.observe(banners);
    return () => {
      observer.disconnect();
      document.documentElement.style.removeProperty("--banner-height");
    };
  }, []);

  // z-40 keeps the banners above the top nav (z-30) and below the modal layer
  // (z-50), so sheets and dialogs cover them instead of being clipped by them.
  // Modal surfaces are not offset for the banners; never raise them above
  // z-50.
  return (
    <div className="sticky top-0 z-40" ref={ref}>
      <DemoBanner />
      <NewBuildBanner />
    </div>
  );
};

export default TopBanners;
