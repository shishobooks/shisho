import BookItem from "./BookItem";

import { useBulkSelection } from "@/hooks/useBulkSelection";
import type { Book } from "@/types";

interface SelectableBookItemProps {
  book: Book;
  libraryId: string;
  seriesId?: number;
  coverAspectRatio?: string;
  addedByUsername?: string;
  pageBookIds: number[];
}

export const SelectableBookItem = ({
  book,
  libraryId,
  seriesId,
  coverAspectRatio,
  addedByUsername,
  pageBookIds,
}: SelectableBookItemProps) => {
  const { isSelectionMode, isSelected, toggleBook, selectRange } =
    useBulkSelection();

  return (
    <BookItem
      addedByUsername={addedByUsername}
      book={book}
      coverAspectRatio={coverAspectRatio}
      isSelected={isSelected(book.id)}
      isSelectionMode={isSelectionMode}
      libraryId={libraryId}
      onSelect={() => toggleBook(book.id)}
      onShiftSelect={() => selectRange(book.id, pageBookIds)}
      seriesId={seriesId}
    />
  );
};
