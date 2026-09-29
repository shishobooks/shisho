import { useAuth } from "@/hooks/useAuth";
import type { Requirement } from "@/utils/permissions";

/**
 * Whether the signed-in role meets `requirement`, such as
 * `useCan("books:write")`, `useCan(["jobs:read", "jobs:write"])` or
 * `useCan(anyOf("shares:read", "shares:write"))`. Code that cannot call a hook
 * per check uses `can` from `useAuth()`.
 */
export const useCan = (requirement: Requirement): boolean =>
  useAuth().can(requirement);
