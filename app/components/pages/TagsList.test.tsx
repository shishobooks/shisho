import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import { mockCoverDimensions } from "@/testing/coverDimensions";
import type { ListTagsQuery } from "@/types/generated/tags";

import TagsList from "./TagsList";

mockCoverDimensions();

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

const allTags = [
  { id: 1, name: "Full Color" },
  { id: 2, name: "Circle: Moon Factory" },
  { id: 3, name: "Circle:Apple Lab" },
];
const isNamespaced = (name: string) => /^circle:/i.test(name);

const tagItem = (t: { id: number; name: string }) => ({
  ...t,
  library_id: 1,
  book_count: 1,
  aliases: [],
  preview_books: [],
});

let tagQueries: ListTagsQuery[] = [];

const mockServer = (namespaces: string[]) => {
  tagQueries = [];
  const spy = vi.spyOn(API, "request");
  spy.mockClear();
  spy.mockImplementation(async (_method, path, _payload, query) => {
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
    if (path === "/tags/namespaces") {
      return [
        { namespace: null, total: 1 },
        { namespace: "Circle", total: 2 },
      ];
    }
    if (path === "/tags") {
      const q = query as ListTagsQuery;
      tagQueries.push(q);
      const items = allTags.filter((t) =>
        q.namespace
          ? isNamespaced(t.name)
          : q.exclude_namespaces
            ? !isNamespaced(t.name)
            : true,
      );
      return { items: items.map(tagItem), total: items.length };
    }
    throw new Error(`unexpected request to ${path}`);
  });
};

const LocationSearch = () => (
  <div data-testid="location">{useLocation().search}</div>
);

const renderPage = (entry = "/libraries/1/tags") =>
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter initialEntries={[entry]}>
        <Routes>
          <Route
            element={
              <>
                <TagsList />
                <LocationSearch />
              </>
            }
            path="/libraries/:libraryId/tags"
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );

beforeEach(() => {
  setAuth({ permissions: ["books:read"] });
});

describe("TagsList tag namespaces", () => {
  it("opens on the plain tags, with a tab per namespace and its count", async () => {
    mockServer(["Circle"]);
    renderPage();

    const tabs = await screen.findByRole("tablist");
    await waitFor(() =>
      expect(
        within(tabs)
          .getAllByRole("tab")
          .map((t) => t.textContent),
      ).toEqual(["Tags1", "Circle2"]),
    );
    expect(within(tabs).getByRole("tab", { name: /Tags/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(await screen.findByText("Full Color")).toBeInTheDocument();
    expect(screen.queryByText(/Moon Factory/)).not.toBeInTheDocument();
    // The list never fetches unsplit before the library loads.
    expect(tagQueries.every((q) => q.exclude_namespaces)).toBe(true);
  });

  it("lists a namespace's tags without the prefix and keeps the tab in the URL", async () => {
    mockServer(["Circle"]);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderPage("/libraries/1/tags?page=2");

    await user.click(await screen.findByRole("tab", { name: /Circle/ }));

    expect(await screen.findByText("Moon Factory")).toBeInTheDocument();
    expect(screen.getByText("Apple Lab")).toBeInTheDocument();
    expect(screen.queryByText("Full Color")).not.toBeInTheDocument();
    expect(tagQueries[tagQueries.length - 1]).toMatchObject({
      namespace: "Circle",
    });
    // A new tab starts on its first page.
    expect(screen.getByTestId("location")).toHaveTextContent(
      "?namespace=Circle",
    );
    expect(screen.getByLabelText("Search Circle")).toBeInTheDocument();
  });

  it("opens a namespace tab from a link", async () => {
    mockServer(["Circle"]);
    renderPage("/libraries/1/tags?namespace=Circle");

    expect(await screen.findByText("Moon Factory")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Circle/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("shows every tag with no tabs when the library has no namespaces", async () => {
    mockServer([]);
    renderPage();

    expect(await screen.findByText("Circle: Moon Factory")).toBeInTheDocument();
    expect(screen.getByText("Full Color")).toBeInTheDocument();
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    expect(tagQueries.every((q) => !q.namespace && !q.exclude_namespaces)).toBe(
      true,
    );
    expect(
      vi
        .mocked(API.request)
        .mock.calls.some(([, path]) => path === "/tags/namespaces"),
    ).toBe(false);
  });
});
