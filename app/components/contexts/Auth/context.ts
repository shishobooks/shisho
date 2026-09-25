import { createContext } from "react";

import type { MeResponse } from "@/types";

// The authenticated user is exactly the GET /auth/me response shape. Alias the
// generated type rather than restate it so it can never drift from the backend.
export type AuthUser = MeResponse;

export interface AuthContextValue {
  user: AuthUser | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  needsSetup: boolean;
  demoMode: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  hasPermission: (resource: string, operation: string) => boolean;
  /**
   * Shorthand for `hasPermission(resource, "write")`. Use it to gate mutating
   * controls on the permission the backend route requires (see
   * `writeResourceForEntity` in `@/utils/permissions` for metadata entities).
   * Reflects role permissions only; Demo Mode never affects it.
   */
  canWrite: (resource: string) => boolean;
  hasLibraryAccess: (libraryId: number) => boolean;
  refetch: () => Promise<void>;
  setAuthUser: (user: AuthUser) => void;
}

export const AuthContext = createContext<AuthContextValue | undefined>(
  undefined,
);
