import { describe, expect, it } from "vitest";

import { ALL_PERMISSIONS, authValue, setAuth } from "./auth";

describe("the useAuth test mock", () => {
  it("grants every generated resource's read and write in ALL_PERMISSIONS", () => {
    expect(ALL_PERMISSIONS).toHaveLength(16);
    expect(ALL_PERMISSIONS).toContain("books:write");
    expect(ALL_PERMISSIONS).toContain("shares:read");
  });

  it("checks requirements against the permissions setAuth grants", () => {
    setAuth({ permissions: ["books:read", "jobs:read"] });

    expect(authValue().can("books:read")).toBe(true);
    expect(authValue().can(["jobs:read", "jobs:write"])).toBe(false);
    expect(authValue().user?.permissions).toEqual(["books:read", "jobs:read"]);
  });

  it("resets everything setAuth is not given", () => {
    setAuth({ permissions: ["books:read"], demoMode: true, user: null });
    setAuth();

    expect(authValue().can("books:read")).toBe(false);
    expect(authValue().demoMode).toBe(false);
    expect(authValue().isAuthenticated).toBe(true);
  });
});
