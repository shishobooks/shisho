import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useUserLibraries } from "@/hooks/queries/libraries";
import { useListLists } from "@/hooks/queries/lists";

import LibraryListPicker from "./LibraryListPicker";

const auth = vi.hoisted(() => ({ permissions: new Set<string>() }));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    hasPermission: (resource: string, operation: string) =>
      auth.permissions.has(`${resource}:${operation}`),
  }),
}));

vi.mock("@/hooks/queries/libraries", () => ({ useUserLibraries: vi.fn() }));
vi.mock("@/hooks/queries/lists", () => ({ useListLists: vi.fn() }));

const renderPicker = () =>
  render(
    <MemoryRouter initialEntries={["/libraries/1/books/7"]}>
      <Routes>
        <Route
          element={<LibraryListPicker />}
          path="/libraries/:libraryId/books/:id"
        />
      </Routes>
    </MemoryRouter>,
  );

describe("LibraryListPicker", () => {
  beforeEach(() => {
    auth.permissions = new Set(["books:read"]);
    vi.mocked(useUserLibraries).mockImplementation(
      (options) =>
        ({
          data:
            options?.enabled === false
              ? undefined
              : [{ id: 1, name: "Fiction" }],
        }) as never,
    );
    vi.mocked(useListLists).mockReturnValue({
      data: { items: [], total: 0 },
    } as never);
  });

  it("shows the current library for a role with Books Read, without Libraries Read", () => {
    renderPicker();

    expect(screen.getByRole("button", { name: /Fiction/ })).toBeInTheDocument();
  });

  it("renders nothing and requests no libraries for a role without Books Read", () => {
    auth.permissions = new Set(["shares:read"]);
    vi.mocked(useUserLibraries).mockClear();
    const { container } = renderPicker();

    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByText("Select Library")).not.toBeInTheDocument();
    expect(useUserLibraries).toHaveBeenCalled();
    for (const call of vi.mocked(useUserLibraries).mock.calls) {
      expect(call[0]?.enabled).toBe(false);
    }
  });
});
