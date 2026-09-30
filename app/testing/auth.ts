import { vi } from "vitest";

import type { AuthContextValue, AuthUser } from "@/components/contexts/Auth";
import * as models from "@/types/generated/models";
import { meetsRequirement, type Permission } from "@/utils/permissions";

// A stand-in for `@/hooks/useAuth` in component tests. Mock the module with
// this one, then give each test the role it needs:
//
//   vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
//
//   beforeEach(() => setAuth({ permissions: ["books:read"] }));
//
// The test file and the mock share this module, so `setAuth` changes what
// every `useAuth()` call returns. State carries from one test to the next, so
// set it in `beforeEach` when tests need different roles.

/**
 * Every permission a role can hold, from the generated `Resource*` constants,
 * so a resource added on the backend is granted here too.
 */
export const ALL_PERMISSIONS: readonly Permission[] = Object.entries(models)
  .filter(([name]) => /^Resource[A-Z]/.test(name))
  .flatMap(([, resource]) => [
    `${resource}:${models.OperationRead}` as Permission,
    `${resource}:${models.OperationWrite}` as Permission,
  ]);

const DEFAULT_USER: AuthUser = {
  id: 1,
  username: "reader",
  role_id: 1,
  role_name: "reader",
  permissions: [],
  must_change_password: false,
};

interface AuthOptions {
  /** The role's permissions. Defaults to none. */
  permissions?: readonly Permission[];
  demoMode?: boolean;
  /** The PDF render key from GET /auth/status. Defaults to "200-85". */
  pdfRenderKey?: string;
  /** The signed-in user, or `null` when signed out. */
  user?: Partial<AuthUser> | null;
}

const newState = ({
  permissions = [],
  demoMode = false,
  pdfRenderKey = "200-85",
  user = {},
}: AuthOptions = {}) => ({
  permissions: new Set<string>(permissions),
  demoMode,
  pdfRenderKey,
  user: user === null ? null : { ...DEFAULT_USER, ...user },
  login: vi.fn<AuthContextValue["login"]>(),
  logout: vi.fn<AuthContextValue["logout"]>(),
  refetch: vi.fn<AuthContextValue["refetch"]>(),
  setAuthUser: vi.fn<AuthContextValue["setAuthUser"]>(),
});

/** The mocked auth state. `login`, `logout`, `refetch` and `setAuthUser` are spies. */
export const auth = newState();

/** Replaces the mocked auth state; anything left out takes its default. */
export const setAuth = (options: AuthOptions = {}) => {
  Object.assign(auth, newState(options));
};

/** The `useAuth()` value for the current state. */
export const authValue = (): AuthContextValue => {
  const { permissions, user } = auth;
  const hasPermission = (resource: string, operation: string) =>
    permissions.has(`${resource}:${operation}`);
  return {
    user: user && { ...user, permissions: [...permissions] },
    isLoading: false,
    isAuthenticated: user !== null,
    needsSetup: false,
    demoMode: auth.demoMode,
    pdfRenderKey: auth.pdfRenderKey,
    login: auth.login,
    logout: auth.logout,
    can: (requirement) => meetsRequirement(hasPermission, requirement),
    hasLibraryAccess: (libraryId) =>
      user !== null &&
      (!user.library_access || user.library_access.includes(libraryId)),
    refetch: auth.refetch,
    setAuthUser: auth.setAuthUser,
  };
};

export const useAuth = authValue;
