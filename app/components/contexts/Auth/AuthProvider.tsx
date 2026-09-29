import { useCallback, useEffect, useState, type ReactNode } from "react";

import { API } from "@/libraries/api";
import { queryClient } from "@/libraries/query-client";
import type { StatusResponse } from "@/types";
import { meetsRequirement, type Requirement } from "@/utils/permissions";

import { AuthContext, type AuthUser } from "./context";

interface AuthProviderProps {
  children: ReactNode;
}

const AuthProvider = ({ children }: AuthProviderProps) => {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [needsSetup, setNeedsSetup] = useState(false);
  const [demoMode, setDemoMode] = useState(false);

  const checkAuthStatus = useCallback(async () => {
    try {
      // First check if setup is needed
      const status = await API.request<StatusResponse>("GET", "/auth/status");
      setNeedsSetup(status.needs_setup);
      setDemoMode(status.demo_mode);

      if (status.needs_setup) {
        setIsLoading(false);
        return;
      }

      // Try to get current user
      const userData = await API.request<AuthUser>("GET", "/auth/me");
      setUser(userData);
    } catch {
      // User is not authenticated - this is expected if not logged in
      setUser(null);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    checkAuthStatus();
  }, [checkAuthStatus]);

  const login = useCallback(async (username: string, password: string) => {
    const userData = await API.request<AuthUser>("POST", "/auth/login", {
      username,
      password,
    });
    // Cached queries belong to whoever used this tab before (their
    // accessible libraries, lists, and settings), so start empty.
    queryClient.clear();
    setUser(userData);
    setNeedsSetup(false);
  }, []);

  const logout = useCallback(async () => {
    try {
      await API.request("POST", "/auth/logout");
    } catch {
      // Ignore logout errors
    }
    queryClient.clear();
    setUser(null);
  }, []);

  const can = useCallback(
    (requirement: Requirement) =>
      meetsRequirement(
        (resource, operation) =>
          user?.permissions.includes(`${resource}:${operation}`) ?? false,
        requirement,
      ),
    [user],
  );

  const hasLibraryAccess = useCallback(
    (libraryId: number) => {
      if (!user) return false;
      // If library_access is null/undefined, user has access to all libraries
      if (!user.library_access) {
        return true;
      }
      // Otherwise, check if the library is in the access list
      return user.library_access.includes(libraryId);
    },
    [user],
  );

  const refetch = useCallback(async () => {
    try {
      const userData = await API.request<AuthUser>("GET", "/auth/me");
      setUser(userData);
      setNeedsSetup(false); // If we have a user, setup is complete
    } catch {
      setUser(null);
    }
  }, []);

  const setAuthUser = useCallback((userData: AuthUser) => {
    queryClient.clear();
    setUser(userData);
    setNeedsSetup(false);
  }, []);

  return (
    <AuthContext.Provider
      value={{
        user,
        isLoading,
        isAuthenticated: !!user,
        needsSetup,
        demoMode,
        login,
        logout,
        can,
        hasLibraryAccess,
        refetch,
        setAuthUser,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};

export default AuthProvider;
