import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import TagDetail from "./TagDetail";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

beforeAll(() => {
  window.matchMedia = vi.fn(() => ({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })) as unknown as typeof window.matchMedia;
});

const renderTag = (tagName: string, namespaces: string[]) => {
  vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
    if (path === "/settings/user") return { gallery_size: "m" };
    if (path === "/user/libraries") {
      return [
        {
          id: 1,
          name: "Doujin",
          cover_aspect_ratio: "book",
          organize_file_structure: false,
          tag_namespaces: namespaces,
        },
      ];
    }
    if (path === "/tags/7") {
      return {
        id: 7,
        library_id: 1,
        name: tagName,
        book_count: 0,
        aliases: [],
      };
    }
    return { items: [], total: 0 };
  });
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter initialEntries={["/libraries/1/tags/7"]}>
        <Routes>
          <Route
            element={<TagDetail />}
            path="/libraries/:libraryId/tags/:id"
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

beforeEach(() => {
  vi.restoreAllMocks();
  setAuth({ permissions: ["books:read"] });
});

describe("TagDetail tag namespaces", () => {
  it("titles a namespaced tag by its name, with the namespace above and in the breadcrumbs", async () => {
    renderTag("Circle:Moon Factory", ["Circle"]);

    expect(
      await screen.findByRole("heading", { level: 1, name: "Moon Factory" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { level: 1 }).parentElement?.previousSibling,
    ).toHaveTextContent("Circle");
    expect(screen.getByRole("link", { name: "Circle" })).toHaveAttribute(
      "href",
      "/libraries/1/tags?namespace=Circle",
    );
  });

  it("titles a tag by its full name when the library has no namespaces", async () => {
    renderTag("Circle:Moon Factory", []);

    expect(
      await screen.findByRole("heading", {
        level: 1,
        name: "Circle:Moon Factory",
      }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Circle" }),
    ).not.toBeInTheDocument();
  });
});
