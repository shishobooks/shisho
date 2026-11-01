import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { toast } from "sonner";
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

import { API, ShishoAPIError } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import ListsIndex from "./ListsIndex";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
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

const favoritesTemplate = {
  name: "favorites",
  display_name: "Favorites",
  description: "My favorite books",
  is_ordered: false,
  default_sort: "added_at_desc",
};
const favoritesList = { id: 5, name: "Favorites", permission: "owner" };

// Stubs the Lists page around a Favorites template POST: before it, the
// user has no lists; after it, the server has a Favorites list. Lists and
// templates fetched after the POST wait for `refetched`.
const stubTemplateCreate = ({
  createFails = false,
  refetched = Promise.resolve(),
} = {}) => {
  let created = false;
  const request = vi
    .spyOn(API, "request")
    .mockImplementation(async (method, path) => {
      if (method === "POST" && path === "/lists/templates/favorites") {
        created = true;
        if (createFails)
          throw new ShishoAPIError(
            "You already have a list with this name.",
            "validation_error",
            422,
          );
        return favoritesList;
      }
      if (path === "/lists") {
        if (!created) return { items: [], total: 0 };
        await refetched;
        return { items: [favoritesList], total: 1 };
      }
      if (path === "/lists/templates") {
        if (!created) return [favoritesTemplate];
        await refetched;
        return [];
      }
      if (path === "/books") return { items: [], total: 0 };
      return [];
    });
  const refetchesStarted = () =>
    request.mock.calls.filter(
      ([method, path]) =>
        method === "GET" && (path === "/lists" || path === "/lists/templates"),
    ).length - 2;
  return { refetchesStarted };
};

describe("ListsIndex templates", () => {
  beforeEach(() => {
    setAuth({ permissions: ["books:read"] });
    vi.mocked(toast.success).mockClear();
    vi.mocked(toast.error).mockClear();
  });
  afterEach(() => vi.restoreAllMocks());

  // Until the refetched lists replace the template, a second click would
  // try to create a duplicate list.
  it("keeps a template disabled until the new list replaces it", async () => {
    let releaseRefetch = () => {};
    const refetched = new Promise<void>((resolve) => {
      releaseRefetch = resolve;
    });
    const { refetchesStarted } = stubTemplateCreate({ refetched });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderLists();

    const template = await screen.findByRole("button", { name: /Favorites/ });
    await user.click(template);

    await waitFor(() => expect(refetchesStarted()).toBe(2));
    // Give a mutation that didn't wait for the refetches time to settle.
    await act(() => vi.advanceTimersByTimeAsync(50));
    expect(template).toBeDisabled();
    expect(toast.success).not.toHaveBeenCalled();

    releaseRefetch();
    expect(
      await screen.findByRole("link", { name: /Favorites/ }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Favorites/ }),
    ).not.toBeInTheDocument();
    expect(toast.success).toHaveBeenCalledWith('Created "Favorites" list');
  });

  // A page opened before Favorites was created elsewhere still offers the
  // template; the failed create refreshes it to show the existing list.
  it("replaces a stale template with the existing list when creating fails", async () => {
    stubTemplateCreate({ createFails: true });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderLists();

    await user.click(await screen.findByRole("button", { name: /Favorites/ }));

    expect(
      await screen.findByRole("link", { name: /Favorites/ }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Favorites/ }),
    ).not.toBeInTheDocument();
    expect(toast.error).toHaveBeenCalledWith(
      "You already have a list with this name.",
      undefined,
    );
  });
});
