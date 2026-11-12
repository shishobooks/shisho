import type { ReactNode } from "react";
import { Link } from "react-router-dom";

import TruncatedText from "@/components/library/TruncatedText";
import { Badge } from "@/components/ui/badge";
import type { ReadThroughEntry } from "@/types";

// A Read-through's title, authors, and a line of details, as Reading History
// and Reading Stats list them. The title links to the Book only while the
// user can reach it, and a badge says why it doesn't.
const ReadThroughDetails = ({
  entry,
  children,
}: {
  entry: ReadThroughEntry;
  children: ReactNode;
}) => {
  const title = (
    <TruncatedText className="truncate">{entry.title}</TruncatedText>
  );
  return (
    <div className="min-w-0 flex-1">
      {/* TruncatedText renders a div, which a <p> can't hold. */}
      <div className="min-w-0 text-sm font-medium">
        {entry.book ? (
          <Link
            className="block min-w-0 hover:underline"
            to={`/libraries/${entry.book.library_id}/books/${entry.book.id}`}
          >
            {title}
          </Link>
        ) : (
          title
        )}
      </div>
      {entry.authors.length > 0 && (
        <p className="truncate text-xs text-muted-foreground">
          {entry.authors.join(", ")}
        </p>
      )}
      <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
        {children}
        {entry.removed && (
          <Badge className="text-[11px]" variant="outline">
            Removed from library
          </Badge>
        )}
        {!entry.removed && !entry.book && (
          <Badge className="text-[11px]" variant="outline">
            No access to this library
          </Badge>
        )}
      </div>
    </div>
  );
};

export default ReadThroughDetails;
