import type { ReactNode } from "react";

import LibrarySidebar from "@/components/library/LibrarySidebar";
import TopNav from "@/components/library/TopNav";

interface LibraryLayoutProps {
  children: ReactNode;
  /** Maximum width class for content area. Defaults to max-w-7xl */
  maxWidth?: string;
}

const LibraryLayout = ({
  children,
  maxWidth = "max-w-7xl",
}: LibraryLayoutProps) => {
  return (
    <div className="flex flex-col min-h-screen">
      <TopNav />
      <div className="flex flex-1">
        {/* Desktop sidebar - hidden on mobile */}
        <div className="hidden md:block">
          <LibrarySidebar />
        </div>
        <main
          className={`flex-1 w-full mx-auto px-4 py-4 md:px-6 md:py-8 ${maxWidth}`}
        >
          {children}
        </main>
      </div>
    </div>
  );
};

export default LibraryLayout;
