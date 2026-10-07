import { createContext } from "react";

import type { MeResponse } from "@/types";
import type { Requirement } from "@/utils/permissions";

// The authenticated user is exactly the GET /auth/me response shape. Alias the
// generated type rather than restate it so it can never drift from the backend.
export type AuthUser = MeResponse;

export interface AuthContextValue {
  user: AuthUser | null;
  isLoading: boolean;
  isAuthenticated: boolean;
  needsSetup: boolean;
  /**
   * From the unauthenticated GET /auth/status. Branch on this, never on the
   * hostname or username.
   */
  demoMode: boolean;
  /**
   * The server's PDF render settings key from GET /auth/status. PDF page
   * URLs carry it; build them with `useFilePageUrl`.
   */
  pdfRenderKey: string;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  /**
   * Whether the role meets `requirement`: `can("books:write")`, an array the
   * role must hold all of, or `anyOf(...)`. Use it where a hook cannot run
   * per check (after an early return, in a loop or callback); otherwise use
   * `useCan`. Reflects role permissions only; Demo Mode never affects it.
   */
  can: (requirement: Requirement) => boolean;
  hasLibraryAccess: (libraryId: number) => boolean;
  refetch: () => Promise<void>;
  setAuthUser: (user: AuthUser) => void;
}

export const AuthContext = createContext<AuthContextValue | undefined>(
  undefined,
);
