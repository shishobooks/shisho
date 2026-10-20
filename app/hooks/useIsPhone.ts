import { useMediaQuery } from "@/hooks/useMediaQuery";

// Below Tailwind's `sm` breakpoint (640px), the width the layouts treat as a
// phone. Keep in step with the `sm:` classes those layouts use.
const PHONE_QUERY = "(max-width: 639px)";

/** Whether the window is below the `sm` breakpoint. */
export const useIsPhone = (): boolean => useMediaQuery(PHONE_QUERY);
