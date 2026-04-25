import { useBulkSelection } from "@/hooks/useBulkSelection";
import type { Book, GallerySize } from "@/types";

import BookItem from "./BookItem";

interface SelectableBookItemProps {
  book: Book;
  libraryId: string;
  seriesId?: number;
  coverAspectRatio?: string;
  addedByUsername?: string;
  pageBookIds: number[];
  cacheKey?: number;
  gallerySize?: GallerySize;
}

export const SelectableBookItem = ({
  book,
  libraryId,
  seriesId,
  coverAspectRatio,
  addedByUsername,
  pageBookIds,
  cacheKey,
  gallerySize,
}: SelectableBookItemProps) => {
  const { isSelectionMode, isSelected, toggleBook, selectRange } =
    useBulkSelection();

  return (
    <BookItem
      addedByUsername={addedByUsername}
      book={book}
      cacheKey={cacheKey}
      coverAspectRatio={coverAspectRatio}
      gallerySize={gallerySize}
      isSelected={isSelected(book.id)}
      isSelectionMode={isSelectionMode}
      libraryId={libraryId}
      onSelect={() => toggleBook(book.id)}
      onShiftSelect={() => selectRange(book.id, pageBookIds)}
      seriesId={seriesId}
    />
  );
};
