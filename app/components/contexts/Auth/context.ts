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
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  hasPermission: (resource: string, operation: string) => boolean;
  hasLibraryAccess: (libraryId: number) => boolean;
  refetch: () => Promise<void>;
  setAuthUser: (user: AuthUser) => void;
}

export const AuthContext = createContext<AuthContextValue | undefined>(
  undefined,
);
