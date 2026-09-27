import { renderHook } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import { useAuth } from "@/hooks/useAuth";

import { useAdminNavItems } from "./useAdminNavItems";

vi.mock("@/hooks/useAuth", () => ({
  useAuth: vi.fn(),
}));

const sharingItem = (permissions: string[]) => {
  vi.mocked(useAuth).mockReturnValue({
    hasPermission: (resource: string, operation: string) =>
      permissions.includes(`${resource}:${operation}`),
  } as ReturnType<typeof useAuth>);
  const { result } = renderHook(() => useAdminNavItems(), {
    wrapper: ({ children }) => (
      <MemoryRouter initialEntries={["/settings/sharing"]}>
        {children}
      </MemoryRouter>
    ),
  });
  return result.current.find((item) => item.to === "/settings/sharing");
};

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
});
