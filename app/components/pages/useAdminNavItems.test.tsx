import { renderHook } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";
import type { Permission } from "@/utils/permissions";

import { useAdminNavItems } from "./useAdminNavItems";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const navItem = (to: string, permissions: Permission[]) => {
  setAuth({ permissions });
  const { result } = renderHook(() => useAdminNavItems(), {
    wrapper: ({ children }) => (
      <MemoryRouter initialEntries={[to]}>{children}</MemoryRouter>
    ),
  });
  return result.current.find((item) => item.to === to);
};

const sharingItem = (permissions: Permission[]) =>
  navItem("/admin/sharing", permissions);

describe("useAdminNavItems", () => {
  it("shows the Sharing page to users with Config Read", () => {
    const item = sharingItem(["config:read"]);
    expect(item).toMatchObject({
      label: "Sharing",
      show: true,
      isActive: true,
    });
  });

  it("hides the Sharing page without Config Read", () => {
    expect(sharingItem(["shares:read", "shares:write"])?.show).toBe(false);
  });

  it("shows Server Stats to users with Config Read", () => {
    expect(navItem("/admin/server-stats", ["config:read"])).toMatchObject({
      label: "Server Stats",
      show: true,
      isActive: true,
    });
  });

  it("hides Server Stats from users without Config Read", () => {
    expect(
      navItem("/admin/server-stats", [
        "books:read",
        "users:read",
        "libraries:read",
        "jobs:read",
      ])?.show,
    ).toBe(false);
  });

  it("does not mark Server Settings active on Server Stats", () => {
    setAuth({ permissions: ["config:read"] });
    const { result } = renderHook(() => useAdminNavItems(), {
      wrapper: ({ children }) => (
        <MemoryRouter initialEntries={["/admin/server-stats"]}>
          {children}
        </MemoryRouter>
      ),
    });
    expect(
      result.current.find((item) => item.to === "/admin/server")?.isActive,
    ).toBe(false);
  });
});
