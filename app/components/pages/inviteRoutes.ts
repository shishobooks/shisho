import type { RouteObject } from "react-router-dom";

import AcceptInvite from "@/components/pages/AcceptInvite";

// The public Invite Link welcome page. It sits outside the protected tree, so
// someone without an account is never sent to sign in. The splat catches a
// truncated link, which then shows the no-longer-valid page instead of the
// router's error page.
export const inviteRoutes: RouteObject[] = [
  { path: "/invite/:token", Component: AcceptInvite },
  { path: "/invite/*", Component: AcceptInvite },
];
