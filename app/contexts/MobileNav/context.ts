import { createContext } from "react";

export interface MobileNavContextValue {
  isOpen: boolean;
  open: () => void;
  close: () => void;
  toggle: () => void;
}

export const MobileNavContext = createContext<MobileNavContextValue | null>(
  null,
);
