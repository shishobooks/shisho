import { Navigate } from "react-router-dom";

import { AccessDenied } from "@/components/library/ProtectedRoute";

import { useAdminNavItems } from "./useAdminNavItems";

// /settings has no page of its own. It opens the first settings page the
// role may see, so the gear never lands a role without Config Read on
// Access Denied.
const SettingsIndexRedirect = () => {
  const first = useAdminNavItems().find((item) => item.show);
  if (!first) {
    return <AccessDenied />;
  }
  return <Navigate replace to={first.to} />;
};

export default SettingsIndexRedirect;
