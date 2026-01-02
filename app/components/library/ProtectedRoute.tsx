import { Loader2 } from "lucide-react";
import type { ReactNode } from "react";
import { Navigate, useParams } from "react-router-dom";

import { useAuth } from "@/hooks/useAuth";

interface ProtectedRouteProps {
  children: ReactNode;
  requiredPermission?: {
    resource: string;
    operation: string;
  };
  checkLibraryAccess?: boolean; // If true, checks libraryId param against user's library access
}

const ProtectedRoute = ({
  checkLibraryAccess,
  children,
  requiredPermission,
}: ProtectedRouteProps) => {
  const {
    isAuthenticated,
    isLoading,
    needsSetup,
    hasPermission,
    hasLibraryAccess,
  } = useAuth();
  const params = useParams();

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  // Redirect to setup if needed
  if (needsSetup) {
    return <Navigate replace to="/setup" />;
  }

  // Redirect to login if not authenticated
  if (!isAuthenticated) {
    return <Navigate replace to="/login" />;
  }

  // Check permission if required
  if (
    requiredPermission &&
    !hasPermission(requiredPermission.resource, requiredPermission.operation)
  ) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <div className="text-center">
          <h1 className="text-2xl font-semibold mb-2">Access Denied</h1>
          <p className="text-muted-foreground">
            You don't have permission to access this page.
          </p>
        </div>
      </div>
    );
  }

  // Check library access if required
  if (checkLibraryAccess && params.libraryId) {
    const libraryId = parseInt(params.libraryId, 10);
    if (!isNaN(libraryId) && !hasLibraryAccess(libraryId)) {
      return (
        <div className="min-h-screen flex items-center justify-center bg-background">
          <div className="text-center">
            <h1 className="text-2xl font-semibold mb-2">Access Denied</h1>
            <p className="text-muted-foreground">
              You don't have access to this library.
            </p>
          </div>
        </div>
      );
    }
  }

  return <>{children}</>;
};

export default ProtectedRoute;
