import { createContext } from "react";

export interface GettingStartedPanelContextValue {
  isOpen: boolean;
  setOpen: (open: boolean) => void;
  /** Whether the nav pill should pulse once to draw the eye to it. */
  pulse: boolean;
  /** Pulses the pill once, on the next page that shows it. */
  requestPulse: () => void;
  /** Called by the pill once it has pulsed. */
  clearPulse: () => void;
}

export const GettingStartedPanelContext =
  createContext<GettingStartedPanelContextValue | null>(null);
