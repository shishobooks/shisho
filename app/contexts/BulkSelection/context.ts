import { createContext } from "react";

export interface BulkSelectionContextValue {
  selectedBookIds: number[];
  isSelectionMode: boolean;
  enterSelectionMode: () => void;
  exitSelectionMode: () => void;
  toggleBook: (bookId: number) => void;
  selectRange: (toBookId: number, pageBookIds: number[]) => void;
  clearSelection: () => void;
  isSelected: (bookId: number) => boolean;
  selectAll: (bookIds: number[]) => void;
}

export const BulkSelectionContext =
  createContext<BulkSelectionContextValue | null>(null);
