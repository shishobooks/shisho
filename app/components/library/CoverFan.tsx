import CoverImage from "@/components/library/CoverImage";
import { cn } from "@/libraries/utils";
import type { CoverAspectRatio, PreviewBook, ThumbnailAspect } from "@/types";
import { bookCoverUrl } from "@/utils/coverUrl";

// Covers are 56px tall: portrait 2:3 like the gallery's book covers, or
// square like its audiobook covers. Each later cover sits STEP_PX to the
// right of the one before.
const BOX: Record<ThumbnailAspect, { className: string; widthPx: number }> = {
  book: { className: "h-14 w-[38px]", widthPx: 38 },
  square: { className: "h-14 w-14", widthPx: 56 },
};
const STEP_PX = 12;
const MAX_COVERS = 3;

// The slot is always as wide as a full fan of the widest covers the library
// can show, so the text beside it starts at the same place on every entry.
const slotWidthPx = (libraryCoverAspectRatio: CoverAspectRatio) => {
  const widest = libraryCoverAspectRatio === "book" ? "book" : "square";
  return BOX[widest].widthPx + (MAX_COVERS - 1) * STEP_PX;
};

const Placeholder = ({
  aspect,
  left = 0,
  zIndex,
}: {
  aspect: ThumbnailAspect;
  left?: number;
  zIndex?: number;
}) => (
  <div
    className={cn(
      "absolute top-0 rounded-sm border border-dashed border-border bg-muted/30",
      BOX[aspect].className,
    )}
    data-testid="cover-fan-placeholder"
    style={{ left, zIndex }}
  />
);

interface CoverFanProps {
  books: PreviewBook[];
  /** The library's cover preference, which sizes the slot. */
  libraryCoverAspectRatio: CoverAspectRatio;
}

/**
 * Up to three book covers fanned out in a fixed-width slot, newest first on
 * top, each in the shape the gallery gives it. Decorative: the entry's name
 * says what it is.
 */
const CoverFan = ({ books, libraryCoverAspectRatio }: CoverFanProps) => (
  <div
    aria-hidden="true"
    className="relative h-14 shrink-0"
    style={{ width: slotWidthPx(libraryCoverAspectRatio) }}
  >
    {books.length === 0 && (
      <Placeholder
        aspect={
          libraryCoverAspectRatio.startsWith("audiobook") ? "square" : "book"
        }
      />
    )}
    {books.slice(0, MAX_COVERS).map((book, index) => {
      const left = index * STEP_PX;
      // A book with no cover file has no cache key and no cover to serve.
      if (!book.cover_cache_key) {
        return (
          <Placeholder
            aspect={book.cover_aspect}
            key={book.id}
            left={left}
            zIndex={MAX_COVERS - index}
          />
        );
      }
      const src = bookCoverUrl(book);
      return (
        <CoverImage
          alt=""
          className={cn(
            // A border in the page color cuts a thin gap between overlapping
            // covers, so the fan reads as separate books.
            "absolute top-0 rounded-sm border border-background object-cover shadow-md",
            BOX[book.cover_aspect].className,
          )}
          key={src}
          src={src}
          style={{ left, zIndex: MAX_COVERS - index }}
        />
      );
    })}
  </div>
);

export default CoverFan;
