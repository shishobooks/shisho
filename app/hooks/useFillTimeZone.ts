import { useEffect, useRef } from "react";

import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { useAuth } from "@/hooks/useAuth";
import { browserTimeZone } from "@/utils/timeZone";

// Fills the signed-in user's time zone from the browser while it's unset,
// since it dates automatic Read-through days. A zone the user chose in
// Preferences stays. Demo Mode keeps settings in the browser, so it skips.
export const useFillTimeZone = () => {
  const { isAuthenticated, demoMode } = useAuth();
  const enabled = isAuthenticated && !demoMode;
  const settings = useUserSettings({ enabled });
  const updateSettings = useUpdateUserSettings();
  const filledRef = useRef(false);

  const unset =
    enabled && settings.data !== undefined && !settings.data.time_zone;
  useEffect(() => {
    if (!unset || filledRef.current) return;
    const zone = browserTimeZone();
    if (!zone) return;
    filledRef.current = true;
    // A failure only means the next page load tries again.
    updateSettings.mutate({ time_zone: zone }, { onError: () => {} });
  }, [unset, updateSettings]);
};
