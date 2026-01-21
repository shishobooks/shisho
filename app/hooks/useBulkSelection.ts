import { useContext } from "react";

import {
  BulkSelectionContext,
  type BulkSelectionContextValue,
} from "@/contexts/BulkSelection";

export const useBulkSelection = (): BulkSelectionContextValue => {
  const context = useContext(BulkSelectionContext);
  if (!context) {
    throw new Error(
      "useBulkSelection must be used within a BulkSelectionProvider",
    );
  }
  return context;
};
