import { useContext } from "react";

import { AuthContext, type AuthContextValue } from "@/components/contexts/Auth";

export const useAuth = (): AuthContextValue => {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
};
