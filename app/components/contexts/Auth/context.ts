import { createContext } from "react";

export interface AuthUser {
  id: number;
  username: string;
  email?: string;
  role_id: number;
  role_name: string;
  permissions: string[];
  library_access?: number[] | null; // null/undefined = all libraries, empty = none, populated = specific libraries
  must_change_password: boolean;
}

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
