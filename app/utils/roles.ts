import type { Role } from "@/types";

/**
 * Sorts roles with system roles first (admin, editor, viewer)
 * followed by custom roles in alphabetical order.
 */
export function sortRoles(roles: Role[]): Role[] {
  return [...roles].sort((a, b) => {
    const order: Record<string, number> = { admin: 0, editor: 1, viewer: 2 };
    const aOrder = order[a.name.toLowerCase()] ?? 99;
    const bOrder = order[b.name.toLowerCase()] ?? 99;
    if (aOrder !== bOrder) return aOrder - bOrder;
    return a.name.localeCompare(b.name);
  });
}
