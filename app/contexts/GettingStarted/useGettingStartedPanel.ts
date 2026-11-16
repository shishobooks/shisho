import { useContext } from "react";

import { GettingStartedPanelContext } from "./context";

export const useGettingStartedPanel = () => {
  const context = useContext(GettingStartedPanelContext);
  if (!context) {
    throw new Error(
      "useGettingStartedPanel must be used within a GettingStartedPanelProvider",
    );
  }
  return context;
};
