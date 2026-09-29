import type { EntityType } from "@/libraries/metadataEntity";
import type { OperationRead, OperationWrite } from "@/types";
import type * as models from "@/types/generated/models";

type Models = typeof models;

/**
 * Every permission resource, from the generated `Resource*` constants, so a
 * resource added on the backend joins this type with no change here.
 */
type Resource = {
  [K in keyof Models]: K extends `Resource${string}` ? Models[K] : never;
}[keyof Models];

type Operation = typeof OperationRead | typeof OperationWrite;

/** A role permission in the backend's `resource:operation` form. */
export type Permission = `${Resource}:${Operation}`;

/**
 * The permission a backend route requires: one permission, every permission
 * in a list, or any one of a list (`anyOf(...)`) for a route that accepts
 * several.
 */
export type Requirement =
  | Permission
  | readonly Permission[]
  | { readonly anyOf: readonly Permission[] };

/** A requirement met by holding any one of `permissions`. */
export const anyOf = (...permissions: Permission[]): Requirement => ({
  anyOf: permissions,
});

type HasPermission = (resource: string, operation: string) => boolean;

const holds = (hasPermission: HasPermission, permission: Permission) => {
  const [resource, operation] = permission.split(":");
  return hasPermission(resource, operation);
};

/**
 * The permission the backend requires for mutating a metadata entity. Genres,
 * tags, and publishers are edited under the books route group, so they
 * require Books Write rather than a resource of their own.
 */
const WRITE_PERMISSION_BY_ENTITY: Record<EntityType, Permission> = {
  genre: "books:write",
  tag: "books:write",
  publisher: "books:write",
  series: "series:write",
  person: "people:write",
};

export function writePermissionForEntity(entityType: EntityType): Permission {
  return WRITE_PERMISSION_BY_ENTITY[entityType];
}

/**
 * Whether a role with `hasPermission` meets `requirement`. Components use
 * `useCan` or `can` from `useAuth()`, which are built on this.
 */
export const meetsRequirement = (
  hasPermission: HasPermission,
  requirement: Requirement,
): boolean => {
  if (typeof requirement === "string") {
    return holds(hasPermission, requirement);
  }
  if ("anyOf" in requirement) {
    return requirement.anyOf.some((p) => holds(hasPermission, p));
  }
  return (requirement as readonly Permission[]).every((p) =>
    holds(hasPermission, p),
  );
};

/**
 * The permission each page's route guard requires. `router.tsx` guards the
 * routes with these and the navigation hooks show an entry only when the role
 * meets its page's requirement, so a link never leads to Access Denied.
 */
export const ROUTE_PERMISSIONS = {
  libraryBooks: "books:read",
  librarySeries: ["books:read", "series:read"],
  libraryPeople: ["books:read", "people:read"],
  librarySettings: ["libraries:read", "libraries:write"],
  createLibrary: "libraries:write",
  settingsConfig: "config:read",
  settingsLibraries: "libraries:read",
  settingsUsers: "users:read",
  settingsCreateUser: "users:write",
  settingsJobs: "jobs:read",
} as const satisfies Record<string, Requirement>;
