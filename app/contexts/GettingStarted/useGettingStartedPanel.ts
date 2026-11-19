import { useContext, useState } from "react";

import {
  GettingStartedPanelContext,
  type GettingStartedPanelContextValue,
} from "./context";

const noop = () => {};

/**
 * Whether the Getting Started panel is open. Outside a provider, such as a
 * nav rendered alone in a test, the state is the caller's own and the pill
 * never pulses.
 */
export const useGettingStartedPanel = (): GettingStartedPanelContextValue => {
  const context = useContext(GettingStartedPanelContext);
  const [isOpen, setOpen] = useState(false);
  return (
    context ?? {
      isOpen,
      setOpen,
      pulse: false,
      requestPulse: noop,
      clearPulse: noop,
    }
  );
};
