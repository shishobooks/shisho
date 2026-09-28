import { Fragment } from "react";
import { Link } from "react-router-dom";

import { useAuth } from "@/hooks/useAuth";

interface BreadcrumbItem {
  label: string;
  to?: string;
}

interface LibraryBreadcrumbsProps {
  libraryId: string;
  libraryName?: string;
  items: BreadcrumbItem[];
}

// The library crumb shows the caller's libraryName, or a "Library"
// placeholder while a role with Libraries Read waits for it. A role without
// Libraries Read cannot fetch the library, so it sees the crumb only when the
// caller found the name in data the role can read (a book carries its
// library); otherwise the crumb is dropped rather than left generic.
const LibraryBreadcrumbs = ({
  libraryId,
  libraryName,
  items,
}: LibraryBreadcrumbsProps) => {
  const { hasPermission } = useAuth();
  const showLibrary =
    Boolean(libraryName) || hasPermission("libraries", "read");

  return (
    <nav className="mb-4 text-xs sm:text-sm text-muted-foreground overflow-hidden">
      <ol className="flex items-center gap-1 sm:gap-2 flex-wrap">
        {showLibrary && (
          <li className="shrink-0">
            <Link
              className="hover:text-foreground hover:underline"
              to={`/libraries/${libraryId}`}
            >
              {libraryName || "Library"}
            </Link>
          </li>
        )}
        {items.map((item, i) => (
          <Fragment key={i}>
            {(showLibrary || i > 0) && (
              <li aria-hidden="true" className="shrink-0">
                ›
              </li>
            )}
            {item.to ? (
              <li className="shrink-0">
                <Link
                  className="hover:text-foreground hover:underline"
                  to={item.to}
                >
                  {item.label}
                </Link>
              </li>
            ) : (
              <li className="text-foreground truncate">{item.label}</li>
            )}
          </Fragment>
        ))}
      </ol>
    </nav>
  );
};

export default LibraryBreadcrumbs;
