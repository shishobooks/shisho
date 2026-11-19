import { useCallback, useMemo, useState, type ReactNode } from "react";

import { GettingStartedPanelContext } from "./context";

/**
 * Holds whether the Getting Started panel is open, so the user menu's
 * "Getting Started" entry can open the panel the nav pill anchors. Root
 * provides it above every page: each page renders its own top nav, and `/`
 * redirects after rendering one, so state kept in a nav would be lost with
 * it. It also carries Setup's request to pulse the pill once on the page
 * Setup lands on, for the same reason.
 */
export const GettingStartedPanelProvider = ({
  children,
}: {
  children: ReactNode;
}) => {
  const [isOpen, setOpen] = useState(false);
  const [pulse, setPulse] = useState(false);
  const requestPulse = useCallback(() => setPulse(true), []);
  const clearPulse = useCallback(() => setPulse(false), []);
  const value = useMemo(
    () => ({ isOpen, setOpen, pulse, requestPulse, clearPulse }),
    [isOpen, pulse, requestPulse, clearPulse],
  );
  return (
    <GettingStartedPanelContext.Provider value={value}>
      {children}
    </GettingStartedPanelContext.Provider>
  );
};
