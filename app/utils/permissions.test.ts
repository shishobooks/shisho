import { describe, expect, it } from "vitest";

import {
  anyOf,
  meetsRequirement,
  writePermissionForEntity,
  type Permission,
} from "./permissions";

describe("writePermissionForEntity", () => {
  // Mirrors the permission each backend mutation route requires: genres,
  // tags, and publishers live under the books group.
  it.each([
    ["genre", "books:write"],
    ["tag", "books:write"],
    ["publisher", "books:write"],
    ["series", "series:write"],
    ["person", "people:write"],
  ] as const)("maps %s to %s", (entity, permission) => {
    expect(writePermissionForEntity(entity)).toBe(permission);
  });
});

describe("Permission", () => {
  it("is built from the generated resources, so a typo fails to compile", () => {
    const known: Permission = "shares:write";
    // @ts-expect-error "book" is not a generated resource.
    const typo: Permission = "book:read";

    expect([known, typo]).toHaveLength(2);
  });
});

describe("meetsRequirement", () => {
  const holding =
    (...held: string[]) =>
    (resource: string, operation: string) =>
      held.includes(`${resource}:${operation}`);

  it("needs the one permission", () => {
    expect(meetsRequirement(holding("books:read"), "books:read")).toBe(true);
    expect(meetsRequirement(holding("books:read"), "books:write")).toBe(false);
  });

  it("needs every permission in a list", () => {
    const requirement = ["jobs:read", "jobs:write"] as const;
    expect(
      meetsRequirement(holding("jobs:read", "jobs:write"), requirement),
    ).toBe(true);
    expect(meetsRequirement(holding("jobs:read"), requirement)).toBe(false);
  });

  it("needs any one permission of anyOf", () => {
    const requirement = anyOf("shares:read", "shares:write");
    expect(meetsRequirement(holding("shares:write"), requirement)).toBe(true);
    expect(meetsRequirement(holding("books:read"), requirement)).toBe(false);
  });
});
