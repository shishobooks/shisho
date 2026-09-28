import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useLibraries } from "@/hooks/queries/libraries";
import { useListLists } from "@/hooks/queries/lists";

import LibraryListPicker from "./LibraryListPicker";

const auth = vi.hoisted(() => ({ permissions: new Set<string>() }));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({
    hasPermission: (resource: string, operation: string) =>
      auth.permissions.has(`${resource}:${operation}`),
  }),
}));

vi.mock("@/hooks/queries/libraries", () => ({ useLibraries: vi.fn() }));
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
    auth.permissions = new Set(["books:read", "libraries:read"]);
    vi.mocked(useLibraries).mockImplementation(
      (_query, options) =>
        ({
          data:
            options?.enabled === false
              ? undefined
              : { items: [{ id: 1, name: "Fiction" }], total: 1 },
        }) as never,
    );
    vi.mocked(useListLists).mockReturnValue({
      data: { items: [], total: 0 },
    } as never);
  });

  it("shows the current library for a role that can read libraries", () => {
    renderPicker();

    expect(screen.getByRole("button", { name: /Fiction/ })).toBeInTheDocument();
  });

  it("renders nothing and requests no libraries for a role without Libraries Read", () => {
    auth.permissions = new Set(["books:read"]);
    vi.mocked(useLibraries).mockClear();
    const { container } = renderPicker();

    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByText("Select Library")).not.toBeInTheDocument();
    for (const call of vi.mocked(useLibraries).mock.calls) {
      expect(call[1]?.enabled).toBe(false);
    }
  });
});
