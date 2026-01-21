import {
  BulkSelectionContext,
  type BulkSelectionContextValue,
} from "./context";
import { useCallback, useMemo, useRef, useState, type ReactNode } from "react";

interface BulkSelectionProviderProps {
  children: ReactNode;
}

export const BulkSelectionProvider = ({
  children,
}: BulkSelectionProviderProps) => {
  const [selectedBookIds, setSelectedBookIds] = useState<number[]>([]);
  const [isSelectionMode, setIsSelectionMode] = useState(false);
  const lastSelectedIdRef = useRef<number | null>(null);

  const enterSelectionMode = useCallback(() => {
    setIsSelectionMode(true);
  }, []);

  const exitSelectionMode = useCallback(() => {
    setIsSelectionMode(false);
    setSelectedBookIds([]);
    lastSelectedIdRef.current = null;
  }, []);

  const toggleBook = useCallback((bookId: number) => {
    setSelectedBookIds((prev) => {
      if (prev.includes(bookId)) {
        return prev.filter((id) => id !== bookId);
      }
      return [...prev, bookId];
    });
    lastSelectedIdRef.current = bookId;
  }, []);

  const selectRange = useCallback(
    (toBookId: number, pageBookIds: number[]) => {
      if (lastSelectedIdRef.current === null) {
        toggleBook(toBookId);
        return;
      }

      const fromIndex = pageBookIds.indexOf(lastSelectedIdRef.current);
      const toIndex = pageBookIds.indexOf(toBookId);

      if (fromIndex === -1 || toIndex === -1) {
        toggleBook(toBookId);
        return;
      }

      const start = Math.min(fromIndex, toIndex);
      const end = Math.max(fromIndex, toIndex);
      const rangeIds = pageBookIds.slice(start, end + 1);

      setSelectedBookIds((prev) => {
        const newSet = new Set(prev);
        rangeIds.forEach((id) => newSet.add(id));
        return Array.from(newSet);
      });
      lastSelectedIdRef.current = toBookId;
    },
    [toggleBook],
  );

  const clearSelection = useCallback(() => {
    setSelectedBookIds([]);
    lastSelectedIdRef.current = null;
  }, []);

  const isSelected = useCallback(
    (bookId: number) => selectedBookIds.includes(bookId),
    [selectedBookIds],
  );

  const selectAll = useCallback((bookIds: number[]) => {
    setSelectedBookIds((prev) => {
      const newSet = new Set(prev);
      bookIds.forEach((id) => newSet.add(id));
      return Array.from(newSet);
    });
  }, []);

  const value: BulkSelectionContextValue = useMemo(
    () => ({
      selectedBookIds,
      isSelectionMode,
      enterSelectionMode,
      exitSelectionMode,
      toggleBook,
      selectRange,
      clearSelection,
      isSelected,
      selectAll,
    }),
    [
      selectedBookIds,
      isSelectionMode,
      enterSelectionMode,
      exitSelectionMode,
      toggleBook,
      selectRange,
      clearSelection,
      isSelected,
      selectAll,
    ],
  );

  return (
    <BulkSelectionContext.Provider value={value}>
      {children}
    </BulkSelectionContext.Provider>
  );
};
