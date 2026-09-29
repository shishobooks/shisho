import { QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { queryClient } from "@/libraries/query-client";
import { setAuth } from "@/testing/auth";
import type { SharedBookResponse } from "@/types";

import { shareRoutes } from "./shareRoutes";

// A recipient has no session, so every permission check fails.
vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

setAuth({ user: null });

const TOKEN = "q6Ch0Wd2mJ3n9Y1k8Xw4Zr5Tb7Lc0Vf2Hs6Pa3Ne1Mg";

const shared = {
  id: 7,
  created_at: "2026-09-01T00:00:00Z",
  updated_at: "2026-09-01T00:00:00Z",
  library_id: 1,
  library: null,
  filepath: "",
  title: "The Shared Book",
  title_source: "manual",
  sort_title: "Shared Book, The",
  sort_title_source: "manual",
  subtitle: "A Subtitle",
  description: "What the book is about.",
  author_source: "manual",
  cover_cache_key: "42-1",
  authors: [{ id: 1, person_id: 10, person: { id: 10, name: "Ada Author" } }],
  book_series: [
    { id: 1, series_id: 3, series_number: 2, series: { id: 3, name: "Saga" } },
  ],
  book_genres: [{ id: 1, genre_id: 4, genre: { id: 4, name: "Fantasy" } }],
  book_tags: [{ id: 1, tag_id: 6, tag: { id: 6, name: "Favorite" } }],
  files: [
    {
      id: 42,
      created_at: "2026-09-01T00:00:00Z",
      updated_at: "2026-09-02T00:00:00Z",
      book_id: 7,
      library_id: 1,
      file_type: "epub",
      file_role: "main",
      filepath: "",
      filesize_bytes: 1000,
      name: "The Shared Book",
      is_preferred_cover: false,
    },
  ],
  shared_by: "sharer",
  expires_at: "2026-10-04T12:00:00Z",
  cover_aspect_ratio: "book",
} as unknown as SharedBookResponse;

const fetchMock = vi.fn();

// Renders the real share routes, as mounted in app/router.tsx.
const renderPage = (path = `/share/${TOKEN}`) =>
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider
        router={createMemoryRouter(shareRoutes, { initialEntries: [path] })}
      />
    </QueryClientProvider>,
  );

const respondWith = (status: number, body: unknown) =>
  fetchMock.mockResolvedValue(
    new Response(JSON.stringify(body), {
      status,
      headers: { "content-type": "application/json" },
    }),
  );

beforeEach(() => {
  queryClient.clear();
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("__APP_VERSION__", "test");
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("SharedBook", () => {
  it("renders the book with who shared it and when the link expires", async () => {
    respondWith(200, shared);
    renderPage();

    expect(
      await screen.findByRole("heading", { name: "The Shared Book" }),
    ).toBeInTheDocument();
    const notice = screen.getByRole("complementary", { name: "Share details" });
    expect(notice).toHaveTextContent("sharer shared this book with you.");
    expect(notice).toHaveTextContent(/This link expires .*2026/);
    for (const text of [
      "A Subtitle",
      "What the book is about.",
      "Ada Author",
      "Saga",
      "Fantasy",
      "Favorite",
    ]) {
      expect(screen.getByText(text, { exact: false })).toBeInTheDocument();
    }
    expect(screen.getByAltText("The Shared Book Cover")).toHaveAttribute(
      "src",
      `/api/share/${TOKEN}/cover?v=42-1`,
    );
    // The file cover carries the file's updated_at in epoch milliseconds.
    expect(
      document.querySelector(`img[src^="/api/share/${TOKEN}/files/42/cover"]`),
    ).toHaveAttribute(
      "src",
      `/api/share/${TOKEN}/files/42/cover?v=${Date.parse("2026-09-02T00:00:00Z")}`,
    );
    // The file row renders its download button once per layout breakpoint.
    expect(
      screen.getAllByRole("button", { name: "Download" }).length,
    ).toBeGreaterThan(0);
  });

  it("shows no links into the app and no app controls", async () => {
    respondWith(200, shared);
    renderPage();

    await screen.findByRole("heading", { name: "The Shared Book" });
    expect(screen.queryAllByRole("link")).toHaveLength(0);
    expect(screen.queryByLabelText("Book actions")).not.toBeInTheDocument();
    expect(screen.queryByText("Add to list")).not.toBeInTheDocument();
    expect(screen.queryByText("Read")).not.toBeInTheDocument();
    expect(screen.queryByRole("navigation")).not.toBeInTheDocument();
  });

  it("requests only the public share endpoint", async () => {
    respondWith(200, shared);
    renderPage();

    await screen.findByRole("heading", { name: "The Shared Book" });
    const urls = fetchMock.mock.calls.map(([url]) => String(url));
    expect(urls).toEqual([`/api/share/${TOKEN}`]);
  });

  it.each(["/share", "/share/", "/share/a/b"])(
    "shows the unavailable page for the malformed path %s without a request",
    async (path) => {
      renderPage(path);

      expect(
        await screen.findByRole("heading", {
          name: "This link is no longer available",
        }),
      ).toBeInTheDocument();
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it("omits the expiry line for a link that never expires", async () => {
    respondWith(200, { ...shared, expires_at: undefined });
    renderPage();

    await screen.findByRole("heading", { name: "The Shared Book" });
    expect(screen.queryByText(/This link expires/)).not.toBeInTheDocument();
    expect(
      screen.getByRole("complementary", { name: "Share details" }),
    ).toHaveTextContent("sharer shared this book with you.");
  });

  it("shows the one unavailable page when the link cannot be used", async () => {
    respondWith(404, {
      error: { code: "not_found", message: "Share Link not found." },
    });
    renderPage();

    expect(
      await screen.findByRole("heading", {
        name: "This link is no longer available",
      }),
    ).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(/Share Link not found/)).not.toBeInTheDocument();
  });

  it("offers a retry instead of the unavailable page when the server fails", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    fetchMock.mockImplementation(
      async () => new Response("<html>502</html>", { status: 502 }),
    );
    renderPage();

    expect(
      await screen.findByRole(
        "heading",
        { name: "Could not load this link" },
        { timeout: 10000 },
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("This link is no longer available"),
    ).not.toBeInTheDocument();

    fetchMock.mockImplementation(
      async () =>
        new Response(JSON.stringify(shared), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
    );
    await user.click(screen.getByRole("button", { name: "Try again" }));
    expect(
      await screen.findByRole("heading", { name: "The Shared Book" }),
    ).toBeInTheDocument();
  });
});
