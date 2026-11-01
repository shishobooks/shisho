import { useContext } from "react";

import { NavigationHistoryContext } from "./context";

// Null outside the provider (a Share Link page), where no history is known.
export const useNavigationHistory = () => useContext(NavigationHistoryContext);
