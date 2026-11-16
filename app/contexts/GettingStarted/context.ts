import { createContext } from "react";

export interface GettingStartedPanelContextValue {
  isOpen: boolean;
  setOpen: (open: boolean) => void;
}

export const GettingStartedPanelContext =
  createContext<GettingStartedPanelContextValue | null>(null);
