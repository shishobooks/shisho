import type { ReactNode } from "react";
import { Navigate, useLocation, useParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { useAuth } from "@/hooks/useAuth";
import type { Requirement } from "@/utils/permissions";

interface ProtectedRouteProps {
  children: ReactNode;
  // Usually one of ROUTE_PERMISSIONS, so navigation can check the same value.
  requiredPermission?: Requirement;
  checkLibraryAccess?: boolean; // If true, checks libraryId param against user's library access
  unavailableInDemo?: boolean;
}

export const AccessDenied = ({
  message = "You don't have permission to access this page.",
}: {
  message?: string;
}) => (
  <div className="flex min-h-[calc(100vh-var(--demo-banner-height,0px))] items-center justify-center bg-background">
    <div className="text-center">
      <h1 className="text-2xl font-semibold mb-2">Access Denied</h1>
      <p className="text-muted-foreground">{message}</p>
    </div>
  </div>
);

const ProtectedRoute = ({
  checkLibraryAccess,
  children,
  requiredPermission,
  unavailableInDemo,
}: ProtectedRouteProps) => {
  const {
    isAuthenticated,
    isLoading,
    needsSetup,
    demoMode,
    can,
    hasLibraryAccess,
    user,
  } = useAuth();
  const params = useParams();
  const location = useLocation();

  if (isLoading) {
    return (
      <div className="flex min-h-[calc(100vh-var(--demo-banner-height,0px))] items-center justify-center bg-background">
        <LoadingSpinner />
      </div>
    );
  }

  // Redirect to setup if needed
  if (needsSetup) {
    return <Navigate replace to="/setup" />;
  }

  // Redirect to login if not authenticated, preserving the intended destination
  if (!isAuthenticated) {
    const redirectTo = location.pathname + location.search;
    return (
      <Navigate
        replace
        to={`/login?redirect=${encodeURIComponent(redirectTo)}`}
      />
    );
  }

  if (unavailableInDemo && demoMode) {
    return <Navigate replace to="/" />;
  }

  // Force users with temporary passwords to go to security settings
  if (
    !demoMode &&
    user?.must_change_password &&
    location.pathname !== "/user/security"
  ) {
    const redirectTo = location.pathname + location.search;
    return (
      <Navigate
        replace
        to={`/user/security?redirect=${encodeURIComponent(redirectTo)}`}
      />
    );
  }

  // Check permission if required
  if (requiredPermission && !can(requiredPermission)) {
    return <AccessDenied />;
  }

  // Check library access if required
  if (checkLibraryAccess && params.libraryId) {
    const libraryId = parseInt(params.libraryId, 10);
    if (!isNaN(libraryId) && !hasLibraryAccess(libraryId)) {
      return <AccessDenied message="You don't have access to this library." />;
    }
  }

  return <>{children}</>;
};

export default ProtectedRoute;
