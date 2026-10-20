import type { ReactNode } from "react";

import LibrarySidebar from "@/components/library/LibrarySidebar";
import TopNav from "@/components/library/TopNav";
import { ContentWidth } from "@/contexts/ContentWidth";

interface LibraryLayoutProps {
  children: ReactNode;
}

// Content runs the full width beside the sidebar; a page that needs a
// readable measure caps its own parts.
const LibraryLayout = ({ children }: LibraryLayoutProps) => {
  return (
    <div className="flex min-h-[calc(100vh-var(--demo-banner-height,0px))] flex-col">
      <TopNav />
      <div className="flex flex-1">
        {/* Desktop sidebar - hidden on mobile */}
        <div className="hidden md:block">
          <LibrarySidebar />
        </div>
        <main className="min-w-0 flex-1 px-4 py-4 md:px-6 md:py-8">
          <ContentWidth>{children}</ContentWidth>
        </main>
      </div>
    </div>
  );
};

export default LibraryLayout;
