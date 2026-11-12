import { useState } from "react";

import CoverImage from "@/components/library/CoverImage";
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import type { ReadingHistoryBook } from "@/types";
import { bookCoverUrl } from "@/utils/coverUrl";

// A Read-through's Book cover on Reading History and Reading Stats, or a
// placeholder when the Book was deleted, is out of reach, or has no cover.
const ReadThroughCover = ({ book }: { book: ReadingHistoryBook | null }) => {
  const [failed, setFailed] = useState(false);
  const showCover = !!book && !!book.cover_cache_key && !failed;
  return (
    <div className="relative h-16 w-11 shrink-0 overflow-hidden rounded bg-muted">
      {showCover ? (
        <CoverImage
          alt=""
          className="h-full w-full object-cover"
          onError={() => setFailed(true)}
          src={bookCoverUrl(book)}
        />
      ) : (
        <CoverPlaceholder className="absolute inset-0" variant="book" />
      )}
    </div>
  );
};

export default ReadThroughCover;
