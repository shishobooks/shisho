import { Download, Eye, Info, type LucideIcon } from "lucide-react";
import { useLayoutEffect, useRef, type SVGProps } from "react";

import { Button } from "@/components/ui/button";
import { useAuth } from "@/hooks/useAuth";
import { cn } from "@/libraries/utils";

// lucide-react ships no brand icons, so the GitHub mark is inlined.
const GitHubIcon = (props: SVGProps<SVGSVGElement>) => (
  <svg aria-hidden="true" fill="currentColor" viewBox="0 0 24 24" {...props}>
    <path d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12" />
  </svg>
);

interface DemoLink {
  href: string;
  // The accessible name at every width.
  label: string;
  // Visible text per breakpoint, sized so the banner stays on one row. A link
  // with no text below `sm` is icon-only on phones.
  text: { value: string; className: string }[];
  icon: LucideIcon | typeof GitHubIcon;
  variant: "default" | "ghost";
}

const links: DemoLink[] = [
  {
    href: "https://www.shishobooks.com/docs/demo",
    label: "About this library",
    text: [
      { value: "About", className: "hidden sm:inline lg:hidden" },
      { value: "About this library", className: "hidden lg:inline" },
    ],
    icon: Info,
    variant: "ghost",
  },
  {
    href: "https://github.com/shishobooks/shisho",
    label: "GitHub",
    text: [{ value: "GitHub", className: "hidden sm:inline" }],
    icon: GitHubIcon,
    variant: "ghost",
  },
  {
    href: "https://www.shishobooks.com/docs/getting-started",
    label: "Install Shisho",
    text: [
      { value: "Install", className: "sm:hidden" },
      { value: "Install Shisho", className: "hidden sm:inline" },
    ],
    icon: Download,
    variant: "default",
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

  // z-40 keeps the banner above the top nav (z-30) and below the modal layer
  // (z-50), so sheets and dialogs cover it instead of being clipped by it.
  return (
    <aside
      className="sticky top-0 z-40 border-b border-primary/20 bg-background text-sm"
      ref={bannerRef}
    >
      {/* The tint sits on an inner layer so the sticky bar stays opaque. */}
      <div className="bg-primary/10">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-x-4 gap-y-1 px-4 py-1.5 md:px-6">
          <p className="flex min-w-0 items-center gap-2">
            <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/15 text-primary">
              <Eye aria-hidden="true" className="size-3.5" />
            </span>
            <span>
              <span className="font-semibold">Read-only demo.</span>
              <span className="hidden text-muted-foreground md:inline">
                {" "}
                Edits and downloads are disabled.
              </span>
            </span>
          </p>
          <nav aria-label="Demo links" className="flex items-center gap-1">
            {links.map((link) => (
              <Button
                asChild
                className={cn(
                  "h-7 gap-1.5 px-2",
                  link.variant === "default"
                    ? "ml-1 px-2.5"
                    : "text-foreground/80 hover:bg-primary/10 hover:text-foreground",
                )}
                key={link.href}
                size="sm"
                variant={link.variant}
              >
                <a
                  aria-label={link.label}
                  href={link.href}
                  rel="noopener noreferrer"
                  target="_blank"
                >
                  <link.icon aria-hidden="true" />
                  {link.text.map((text) => (
                    <span className={text.className} key={text.value}>
                      {text.value}
                    </span>
                  ))}
                </a>
              </Button>
            ))}
          </nav>
        </div>
      </div>
    </aside>
  );
};

export default DemoBanner;
