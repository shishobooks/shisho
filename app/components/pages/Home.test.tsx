import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { toast } from "sonner";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { API, ShishoAPIError } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import Home from "./Home";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
  window.matchMedia = vi.fn(() => ({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })) as unknown as typeof window.matchMedia;
});

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

vi.mock("@/components/library/SelectionToolbar", () => ({
  SelectionToolbar: () => null,
}));

vi.mock("@/components/library/LibraryLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

const renderHome = (entry = "/libraries/1") => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[entry]}>
        <Routes>
          <Route element={<Home />} path="/libraries/:libraryId" />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
};

const requestedPaths = (request: ReturnType<typeof vi.spyOn>) =>
  request.mock.calls.map((call: unknown[]) => call[1]);

const flush = () =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 50));
  });

describe("Home permission gating", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("requests no books, genres, or tags for a role without Books Read", async () => {
    setAuth({ permissions: ["shares:write"] });
    const request = vi
      .spyOn(API, "request")
      .mockResolvedValue({ items: [], total: 0 });

    renderHome();
    await flush();

    const paths = requestedPaths(request);
    expect(paths).not.toContain("/books");
    expect(paths).not.toContain("/genres");
    expect(paths).not.toContain("/tags");
  });

  it("requests books for a role with Books Read", async () => {
    setAuth({ permissions: ["books:read"] });
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(async (_method, path) =>
        path === "/user/libraries" || path.endsWith("/languages")
          ? []
          : { items: [], total: 0 },
      );

    renderHome();
    await flush();

    expect(requestedPaths(request)).toContain("/books");
  });
});

describe("Home save-as-default failures", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["books:read"] });
    // Every read succeeds and every write fails.
    vi.spyOn(API, "request").mockImplementation(async (method, path) => {
      if (method !== "GET") {
        throw new ShishoAPIError("Settings are read-only", "internal", 500);
      }
      if (path === "/settings/user") return { gallery_size: "m" };
      if (path === "/settings/libraries/1") return { sort_spec: null };
      if (path === "/user/libraries" || path.endsWith("/languages")) return [];
      return { items: [], total: 0 };
    });
  });

  it("toasts when saving the sort as the library default fails", async () => {
    const error = vi.spyOn(toast, "error");
    const user = userEvent.setup();
    renderHome("/libraries/1?sort=title:desc");
    await flush();

    await user.click(screen.getByRole("button", { name: /^sort/i }));
    await user.click(
      await screen.findByRole("button", {
        name: /save as my default for this library/i,
      }),
    );
    await flush();

    expect(error).toHaveBeenCalledWith("Settings are read-only", undefined);
  });

  it("toasts when saving the gallery size as the default fails", async () => {
    const error = vi.spyOn(toast, "error");
    const user = userEvent.setup();
    renderHome("/libraries/1?size=l");
    await flush();

    await user.click(screen.getByRole("button", { name: /^size/i }));
    await user.click(
      await screen.findByRole("button", {
        name: /save as my default everywhere/i,
      }),
    );
    await flush();

    expect(error).toHaveBeenCalledWith("Settings are read-only", undefined);
  });
});

describe("Home search failure", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["books:read"] });
    // The unfiltered list loads; any search fails.
    vi.spyOn(API, "request").mockImplementation(
      async (_method, path, _payload, query) => {
        if (path === "/settings/user") return { gallery_size: "m" };
        if (path === "/settings/libraries/1") return { sort_spec: null };
        if (path === "/user/libraries" || path.endsWith("/languages"))
          return [];
        if (path === "/books" && (query as { search?: string })?.search) {
          throw new ShishoAPIError(
            "Internal Server Error",
            "internal_server_error",
            500,
          );
        }
        return { items: [], total: 0 };
      },
    );
  });

  it("reports a failed search after the unfiltered list loaded", async () => {
    const user = userEvent.setup();
    renderHome();
    expect(
      await screen.findByText("No books in this library yet."),
    ).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText("Search books..."), "zzz");

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /^Failed to load books/,
    );
  });
});

describe("Home active filter chips", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setAuth({ permissions: ["books:read"] });
  });

  it("shows chips for a selected tag and genre missing from the dropdown's first page", async () => {
    // The dropdown lists only the first page of tags and genres, so the
    // selected ones come from their own endpoints.
    vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
      if (path === "/tags/99") return { id: 99, name: "Obscure Tag" };
      if (path === "/genres/77") return { id: 77, name: "Obscure Genre" };
      if (path === "/user/libraries" || path.endsWith("/languages")) return [];
      return { items: [], total: 0 };
    });

    renderHome("/libraries/1?tag_ids=99&genre_ids=77");
    await flush();

    expect(screen.getByText("Obscure Tag")).toBeInTheDocument();
    expect(screen.getByText("Obscure Genre")).toBeInTheDocument();
  });

  it("shows a removable chip for a selected tag or genre that no longer exists", async () => {
    vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
      if (path === "/tags/99" || path === "/genres/77") {
        throw new ShishoAPIError("Not found", "not_found", 404);
      }
      if (path === "/user/libraries" || path.endsWith("/languages")) return [];
      return { items: [], total: 0 };
    });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    renderHome("/libraries/1?tag_ids=99&genre_ids=77");
    await flush();

    expect(screen.getByText("Unknown genre")).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: "Remove Unknown tag" }),
    );
    expect(screen.queryByText("Unknown tag")).not.toBeInTheDocument();
  });

  it("shows a tag deleted since the dropdown loaded as unknown", async () => {
    // The dropdown's list still names the tag, but the tag itself is gone,
    // as after a merge earlier in the session.
    vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
      if (path === "/tags/99") {
        throw new ShishoAPIError("Not found", "not_found", 404);
      }
      if (path === "/tags") {
        return { items: [{ id: 99, name: "Merged Away" }], total: 1 };
      }
      if (path === "/user/libraries" || path.endsWith("/languages")) return [];
      return { items: [], total: 0 };
    });

    renderHome("/libraries/1?tag_ids=99");
    await flush();

    expect(screen.getByText("Unknown tag")).toBeInTheDocument();
    expect(screen.queryByText("Merged Away")).not.toBeInTheDocument();
  });

  it("names a selected tag by id when it fails to load for another reason", async () => {
    vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
      if (path === "/tags/99") {
        throw new ShishoAPIError("Bad request", "bad_request", 400);
      }
      if (path === "/user/libraries" || path.endsWith("/languages")) return [];
      return { items: [], total: 0 };
    });

    renderHome("/libraries/1?tag_ids=99");
    await flush();

    expect(screen.getByText("Tag #99")).toBeInTheDocument();
    expect(screen.queryByText("Unknown tag")).not.toBeInTheDocument();
  });

  it("filters books only by ids in the URL that are positive integers", async () => {
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(async (_method, path) =>
        path === "/user/libraries" || path.endsWith("/languages")
          ? []
          : { items: [], total: 0 },
      );

    renderHome("/libraries/1?tag_ids=abc,0,-1,1.5,5&genre_ids=xyz");
    await flush();

    const booksCall = request.mock.calls.find((call) => call[1] === "/books");
    expect(booksCall?.[3]).toMatchObject({ tag_ids: [5] });
    expect(booksCall?.[3]).not.toHaveProperty("genre_ids");
    const tagPaths = requestedPaths(request).filter((path: string) =>
      path.startsWith("/tags/"),
    );
    expect(tagPaths).toEqual(["/tags/5"]);
  });
});
