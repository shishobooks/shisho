import { sortRoles } from "./roles";
import { describe, expect, it } from "vitest";

import type { Role } from "@/types";

const makeRole = (id: number, name: string, is_system = false): Role => ({
  id,
  name,
  is_system,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
});

describe("sortRoles", () => {
  it("sorts system roles in correct order: admin, editor, viewer", () => {
    const roles = [
      makeRole(1, "Viewer", true),
      makeRole(2, "Admin", true),
      makeRole(3, "Editor", true),
    ];

    const sorted = sortRoles(roles);

    expect(sorted.map((r) => r.name)).toEqual(["Admin", "Editor", "Viewer"]);
  });

  it("is case-insensitive for system role names", () => {
    const roles = [
      makeRole(1, "VIEWER", true),
      makeRole(2, "admin", true),
      makeRole(3, "eDiToR", true),
    ];

    const sorted = sortRoles(roles);

    expect(sorted.map((r) => r.name)).toEqual(["admin", "eDiToR", "VIEWER"]);
  });

  it("places custom roles after system roles", () => {
    const roles = [
      makeRole(1, "Custom Role"),
      makeRole(2, "Admin", true),
      makeRole(3, "Another Custom"),
    ];

    const sorted = sortRoles(roles);

    expect(sorted.map((r) => r.name)).toEqual([
      "Admin",
      "Another Custom",
      "Custom Role",
    ]);
  });

  it("sorts custom roles alphabetically", () => {
    const roles = [
      makeRole(1, "Zebra Role"),
      makeRole(2, "Alpha Role"),
      makeRole(3, "Middle Role"),
    ];

    const sorted = sortRoles(roles);

    expect(sorted.map((r) => r.name)).toEqual([
      "Alpha Role",
      "Middle Role",
      "Zebra Role",
    ]);
  });

  it("handles mixed system and custom roles", () => {
    const roles = [
      makeRole(1, "Custom B"),
      makeRole(2, "Viewer", true),
      makeRole(3, "Custom A"),
      makeRole(4, "Admin", true),
      makeRole(5, "Editor", true),
    ];

    const sorted = sortRoles(roles);

    expect(sorted.map((r) => r.name)).toEqual([
      "Admin",
      "Editor",
      "Viewer",
      "Custom A",
      "Custom B",
    ]);
  });

  it("returns empty array for empty input", () => {
    expect(sortRoles([])).toEqual([]);
  });

  it("does not mutate the original array", () => {
    const roles = [makeRole(1, "Viewer", true), makeRole(2, "Admin", true)];
    const original = [...roles];

    sortRoles(roles);

    expect(roles).toEqual(original);
  });
});
