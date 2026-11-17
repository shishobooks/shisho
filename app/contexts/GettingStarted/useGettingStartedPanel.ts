import { useContext, useState } from "react";

import { GettingStartedPanelContext } from "./context";

/**
 * Whether the Getting Started panel is open. Outside a provider, such as a
 * nav rendered alone in a test, the state is the caller's own.
 */
export const useGettingStartedPanel = () => {
  const context = useContext(GettingStartedPanelContext);
  const [isOpen, setOpen] = useState(false);
  return context ?? { isOpen, setOpen };
};
