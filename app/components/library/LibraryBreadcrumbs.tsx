import { Fragment } from "react";
import { Link } from "react-router-dom";

interface BreadcrumbItem {
  label: string;
  to?: string;
}

interface LibraryBreadcrumbsProps {
  libraryId: string;
  libraryName?: string;
  items: BreadcrumbItem[];
}

const LibraryBreadcrumbs = ({
  libraryId,
  libraryName,
  items,
}: LibraryBreadcrumbsProps) => (
  <nav className="mb-4 text-xs sm:text-sm text-muted-foreground overflow-hidden">
    <ol className="flex items-center gap-1 sm:gap-2 flex-wrap">
      <li className="shrink-0">
        <Link
          className="hover:text-foreground hover:underline"
          to={`/libraries/${libraryId}`}
        >
          {libraryName || "Library"}
        </Link>
      </li>
      {items.map((item, i) => (
        <Fragment key={i}>
          <li aria-hidden="true" className="shrink-0">
            ›
          </li>
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

export default LibraryBreadcrumbs;
