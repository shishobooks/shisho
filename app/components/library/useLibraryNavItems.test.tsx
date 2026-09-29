import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";

import { useLibraryNavItems } from "./useLibraryNavItems";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const shownLabels = () => {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <MemoryRouter initialEntries={["/libraries/1"]}>
      <Routes>
        <Route element={children} path="/libraries/:libraryId" />
      </Routes>
    </MemoryRouter>
  );
  const { result } = renderHook(() => useLibraryNavItems(), { wrapper });
  return (result.current ?? [])
    .filter((item) => item.show)
    .map((item) => item.label);
};

describe("useLibraryNavItems", () => {
  beforeEach(() => {
    setAuth();
  });

  it("hides Series and People without their Read permissions", () => {
    setAuth({ permissions: ["books:read"] });

    expect(shownLabels()).toEqual(["Books", "Genres", "Tags", "Publishers"]);
  });

  it("shows Series and People with their Read permissions", () => {
    setAuth({ permissions: ["books:read", "series:read", "people:read"] });

    expect(shownLabels()).toEqual([
      "Books",
      "Series",
      "People",
      "Genres",
      "Tags",
      "Publishers",
    ]);
  });

  it("shows library Settings only with Libraries Read and Write", () => {
    setAuth({ permissions: ["books:read", "libraries:write"] });
    expect(shownLabels()).not.toContain("Settings");

    setAuth({
      permissions: ["books:read", "libraries:read", "libraries:write"],
    });
    expect(shownLabels()).toContain("Settings");
  });
});
