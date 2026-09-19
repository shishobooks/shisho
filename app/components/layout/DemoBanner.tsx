import { useLayoutEffect, useRef } from "react";

import { useAuth } from "@/hooks/useAuth";

const links = [
  {
    href: "https://www.shishobooks.com/docs/getting-started",
    label: "Install Shisho",
  },
  {
    href: "https://github.com/shishobooks/shisho",
    label: "GitHub",
  },
  {
    href: "https://www.shishobooks.com/docs/demo",
    label: "About this library",
  },
];

const DemoBanner = () => {
  const { demoMode } = useAuth();
  const bannerRef = useRef<HTMLElement>(null);

  useLayoutEffect(() => {
    if (!demoMode || !bannerRef.current) return;

    const banner = bannerRef.current;
    const updateHeight = () => {
      document.documentElement.style.setProperty(
        "--demo-banner-height",
        `${banner.offsetHeight}px`,
      );
    };
    updateHeight();

    const observer = new ResizeObserver(updateHeight);
    observer.observe(banner);
    return () => {
      observer.disconnect();
      document.documentElement.style.removeProperty("--demo-banner-height");
    };
  }, [demoMode]);

  if (!demoMode) return null;

  return (
    <aside
      className="sticky top-0 z-[60] border-b border-primary/20 bg-background px-4 py-2 text-sm"
      ref={bannerRef}
    >
      <div className="mx-auto flex max-w-7xl flex-wrap items-center justify-center gap-x-4 gap-y-1 text-center">
        <span>Read-only demo. Edits and downloads are disabled.</span>
        <nav
          aria-label="Demo links"
          className="flex flex-wrap justify-center gap-3"
        >
          {links.map((link) => (
            <a
              className="font-medium text-primary underline-offset-4 hover:underline"
              href={link.href}
              key={link.href}
              rel="noopener noreferrer"
              target="_blank"
            >
              {link.label}
            </a>
          ))}
        </nav>
      </div>
    </aside>
  );
};

export default DemoBanner;
