import { useMemo, useState, type ReactNode } from "react";

import { GettingStartedPanelContext } from "./context";

/**
 * Holds whether the Getting Started panel is open, so the user menu's
 * "Getting started" entry can open the panel the nav pill anchors. Each top
 * nav that holds both provides it.
 */
export const GettingStartedPanelProvider = ({
  children,
}: {
  children: ReactNode;
}) => {
  const [isOpen, setOpen] = useState(false);
  const value = useMemo(() => ({ isOpen, setOpen }), [isOpen]);
  return (
    <GettingStartedPanelContext.Provider value={value}>
      {children}
    </GettingStartedPanelContext.Provider>
  );
};
