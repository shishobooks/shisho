import { describe, expect, it } from "vitest";

import { writeResourceForEntity } from "./permissions";

describe("writeResourceForEntity", () => {
  // Mirrors the permission each backend mutation route requires: genres,
  // tags, and publishers live under the books group.
  it.each([
    ["genre", "books"],
    ["tag", "books"],
    ["publisher", "books"],
    ["series", "series"],
    ["person", "people"],
  ] as const)("maps %s to %s", (entity, resource) => {
    expect(writeResourceForEntity(entity)).toBe(resource);
  });
});
