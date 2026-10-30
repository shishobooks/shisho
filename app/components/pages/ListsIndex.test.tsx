import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import ListsIndex from "./ListsIndex";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
vi.mock("@/components/library/TopNav", () => ({ default: () => null }));

const stubRequests = ({ lists = [] as unknown[], queued = 3 } = {}) =>
  vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
    if (path === "/lists") return { items: lists, total: lists.length };
    if (path === "/lists/templates") return [];
    if (path === "/books") return { items: [], total: queued };
    return [];
  });

const renderLists = () =>
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter>
        <ListsIndex />
      </MemoryRouter>
    </QueryClientProvider>,
  );

describe("ListsIndex To Read", () => {
  beforeEach(() => setAuth({ permissions: ["books:read"] }));
  afterEach(() => vi.restoreAllMocks());

  it("pins To Read above the lists, with its count", async () => {
    stubRequests({
      lists: [{ id: 5, name: "Favorites", book_count: 2, permission: "owner" }],
    });
    renderLists();

    const toRead = await screen.findByRole("link", { name: /To Read/ });
    expect(toRead).toHaveAttribute("href", "/to-read");
    expect(await within(toRead).findByText("3 books")).toBeInTheDocument();
    const links = screen.getAllByRole("link");
    expect(links[0]).toBe(toRead);
    expect(links[1]).toHaveAttribute("href", "/lists/5");
  });

  it("pins To Read even before any list exists", async () => {
    const request = stubRequests();
    renderLists();

    expect(
      await screen.findByRole("link", { name: /To Read/ }),
    ).toBeInTheDocument();
    expect(request).toHaveBeenCalledWith(
      "GET",
      "/books",
      null,
      { to_read: true, limit: 1 },
      expect.anything(),
    );
  });

  it("leaves To Read out for a role without Books Read", async () => {
    stubRequests();
    setAuth({ permissions: [] });
    renderLists();

    expect(
      await screen.findByText(/You don't have any lists yet/),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: /To Read/ }),
    ).not.toBeInTheDocument();
  });
});
