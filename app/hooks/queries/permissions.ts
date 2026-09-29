import { useAuth } from "@/hooks/useAuth";
import { meetsRequirement, type Requirement } from "@/utils/permissions";

export { anyOf, type Permission, type Requirement } from "@/utils/permissions";

/**
 * ANDs the signed-in role's permissions into a query's `enabled` value.
 * Every query hook whose route needs a permission passes its requirement
 * here and sets the result as `enabled` after spreading caller options, so a
 * caller cannot enable the request for a role that would get a 403:
 *
 *   ...options,
 *   enabled: useRequires("books:read", options.enabled ?? Boolean(id)),
 *
 * `enabled` defaults to true and keeps its type (boolean or TanStack's
 * callback form) when the requirement is met.
 */
export function useRequires<E = boolean>(
  requirement: Requirement,
  enabled: E | true = true,
): E | boolean {
  const { hasPermission } = useAuth();
  return meetsRequirement(hasPermission, requirement) ? enabled : false;
}
