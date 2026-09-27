import type { RouteObject } from "react-router-dom";

import SharedBook from "@/components/pages/SharedBook";

// The public Share Link routes. They sit outside the protected tree, so a
// recipient with no account is never sent to login. The splat catches a
// truncated or mangled link (`/share`, `/share/`, `/share/a/b`), which then
// shows the unavailable page instead of the router's error page.
export const shareRoutes: RouteObject[] = [
  { path: "/share/:token", Component: SharedBook },
  { path: "/share/*", Component: SharedBook },
];
