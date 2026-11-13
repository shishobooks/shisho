import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { toast, Toaster } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AuthProvider } from "@/components/contexts/Auth";
import { MobileNavProvider } from "@/contexts/MobileNav";
import { mockCoverDimensions } from "@/testing/coverDimensions";
import type { ReadingHistoryEntry, ReadThrough } from "@/types";

import ReadingHistoryPage from "./ReadingHistoryPage";

afterEach(() => {
  toast.dismiss();
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const readThrough = (
  id: number,
  overrides: Partial<ReadThrough> = {},
): ReadThrough => ({
  id,
  created_at: "2026-03-01T00:00:00Z",
  updated_at: "2026-03-01T00:00:00Z",
  book_id: id,
  book_title: null,
  book_authors: null,
  begin_day: "2026-03-01",
  end_day: "2026-03-09",
  outcome: "finished",
  paused: false,
  ...overrides,
});

const attached: ReadingHistoryEntry = {
  read_through: readThrough(1),
  title: "Dune",
  authors: ["Frank Herbert"],
  removed: false,
  book: { id: 1, library_id: 7, cover_cache_key: "1-1" },
  can_reread: true,
  session_count: 2,
};

const detachedUnderway: ReadingHistoryEntry = {
  read_through: readThrough(2, {
    book_id: null,
    book_title: "Gone",
    book_authors: ["Ann Author"],
    end_day: undefined,
    outcome: "underway",
  }),
  title: "Gone",
  authors: ["Ann Author"],
  removed: true,
  book: null,
  can_reread: false,
  session_count: 0,
};

const unreachable: ReadingHistoryEntry = {
  read_through: readThrough(3, { outcome: "abandoned" }),
  title: "Elsewhere",
  authors: [],
  removed: false,
  book: null,
  can_reread: false,
  session_count: 0,
};

const renderPage = ({
  entries = [attached, detachedUnderway, unreachable],
  initialEntry = "/reading-history",
}: {
  entries?: ReadingHistoryEntry[];
  initialEntry?: string;
} = {}) => {
  mockCoverDimensions();
  vi.stubGlobal("__APP_VERSION__", "test");
  vi.stubGlobal(
    "matchMedia",
    vi.fn(() => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  );
  const fetchMock = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation(async (input, init) => {
      const path = String(input).split("?")[0];
      if (path === "/api/auth/status")
        return Response.json({ needs_setup: false, demo_mode: false });
      if (path === "/api/auth/me")
        return Response.json({
          id: 1,
          username: "reader",
          permissions: ["books:read"],
          role_name: "viewer",
        });
      if (path === "/api/user/libraries") return Response.json([]);
      if (path === "/api/reading/history")
        return Response.json({ items: entries, total: entries.length });
      if (path === "/api/reading/read-throughs/1/sessions")
        return Response.json({
          time_zone: "America/Chicago",
          items: [
            {
              id: 11,
              started_at: "2026-03-02T01:30:00Z",
              ended_at: "2026-03-02T02:30:00Z",
              seconds: 3600,
              start_percentage: 0,
              end_percentage: 0.2,
              kind: "reading",
              source: "web_reader",
              measured: true,
            },
            {
              id: 12,
              started_at: "2026-03-05T14:00:00Z",
              ended_at: "2026-03-05T14:45:00Z",
              seconds: 1800,
              start_percentage: 0.2,
              end_percentage: 0.3,
              kind: "reading",
              source: "kobo",
              measured: false,
            },
          ],
        });
      if (
        init?.method === "DELETE" &&
        path.startsWith("/api/reading/sessions/")
      )
        return new Response(null, { status: 204 });
      if (init?.method === "DELETE" || init?.method === "POST")
        return Response.json({
          read_through: readThrough(2, { outcome: "finished" }),
          reading: null,
        });
      return Response.json({ items: [], total: 0 });
    });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AuthProvider>
        <MobileNavProvider>
          <MemoryRouter initialEntries={[initialEntry]}>
            <Routes>
              <Route element={<ReadingHistoryPage />} path="/reading-history" />
            </Routes>
            <Toaster />
          </MemoryRouter>
        </MobileNavProvider>
      </AuthProvider>
    </QueryClientProvider>,
  );
  return {
    fetchMock,
    user: userEvent.setup({ advanceTimers: vi.advanceTimersByTime }),
  };
};

const historyRequests = () =>
  vi
    .mocked(globalThis.fetch)
    .mock.calls.map(([input]) => String(input))
    .filter((url) => url.startsWith("/api/reading/history"))
    .map((url) => Object.fromEntries(new URL(url, "http://x").searchParams));

const row = (title: string) =>
  screen.getByText(title).closest("li") as HTMLElement;

describe("ReadingHistoryPage", () => {
  it("links only Books the user can still reach", async () => {
    renderPage();

    const dune = await screen.findByRole("link", { name: "Dune" });
    expect(dune).toHaveAttribute("href", "/libraries/7/books/1");
    expect(screen.queryByRole("link", { name: "Gone" })).toBeNull();
    expect(screen.queryByRole("link", { name: "Elsewhere" })).toBeNull();
    expect(within(row("Gone")).getByText("Removed from library")).toBeVisible();
    expect(
      within(row("Elsewhere")).queryByText("Removed from library"),
    ).toBeNull();
    // A Book out of reach says why it has no link.
    expect(
      within(row("Elsewhere")).getByText("No access to this library"),
    ).toBeVisible();
    expect(
      within(row("Gone")).queryByText("No access to this library"),
    ).toBeNull();
    expect(
      within(row("Dune")).queryByText("No access to this library"),
    ).toBeNull();
    expect(within(row("Dune")).getByText("Frank Herbert")).toBeVisible();
  });

  it("filters by outcome and by removed Books", async () => {
    const { user } = renderPage();
    await screen.findByText("Gone");

    await user.click(screen.getByRole("button", { name: "Did Not Finish" }));
    await waitFor(() =>
      expect(historyRequests()).toContainEqual({
        limit: "50",
        offset: "0",
        outcome: "abandoned",
      }),
    );
    expect(
      screen.getByRole("button", { name: "Did Not Finish" }),
    ).toHaveAttribute("aria-pressed", "true");

    await user.click(
      screen.getByRole("button", { name: "Removed from library" }),
    );
    await waitFor(() =>
      expect(historyRequests()).toContainEqual({
        limit: "50",
        offset: "0",
        outcome: "abandoned",
        removed: "true",
      }),
    );
  });

  it("offers a reread only where the Book can be read again", async () => {
    const { user } = renderPage();
    await screen.findByText("Gone");

    await user.click(screen.getByRole("button", { name: "Actions for Dune" }));
    expect(
      await screen.findByRole("menuitem", { name: "Start a reread" }),
    ).toBeVisible();
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("button", { name: "Actions for Gone" }));
    expect(
      await screen.findByRole("menuitem", { name: "Mark as finished" }),
    ).toBeVisible();
    expect(
      screen.queryByRole("menuitem", { name: "Start a reread" }),
    ).toBeNull();
  });

  it("ends an underway Read-through whose Book was removed", async () => {
    const { fetchMock, user } = renderPage();
    await screen.findByText("Gone");

    await user.click(screen.getByRole("button", { name: "Actions for Gone" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Mark as Did Not Finish" }),
    );

    await waitFor(() => {
      const post = fetchMock.mock.calls.find(
        ([, init]) => init?.method === "POST",
      );
      expect(post?.[0]).toBe("/api/reading/read-throughs/2/abandon");
      expect(JSON.parse(String(post?.[1]?.body))).toHaveProperty("today");
    });
  });

  it("edits dates with the Book page's dialog", async () => {
    const { user } = renderPage();
    await screen.findByText("Gone");

    await user.click(screen.getByRole("button", { name: "Actions for Gone" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "Edit dates" }),
    );

    const dialog = await screen.findByRole("dialog", { name: "Edit dates" });
    expect(within(dialog).getByText("Started")).toBeVisible();
  });

  it("deletes a Read-through after confirming", async () => {
    const { fetchMock, user } = renderPage();
    await screen.findByText("Gone");

    await user.click(screen.getByRole("button", { name: "Actions for Gone" }));
    await user.click(await screen.findByRole("menuitem", { name: "Delete" }));
    const confirm = await screen.findByRole("dialog", {
      name: "Delete this Read-through?",
    });
    expect(
      within(confirm).getByText(/won't count in your reading history/),
    ).toBeVisible();
    await user.click(within(confirm).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      const del = fetchMock.mock.calls.find(
        ([, init]) => init?.method === "DELETE",
      );
      expect(del?.[0]).toBe("/api/reading/read-throughs/2");
    });
  });

  it("lists a Read-through's sessions and deletes one after confirming", async () => {
    const { fetchMock, user } = renderPage();
    await screen.findByText("Dune");
    expect(
      within(row("Gone")).queryByRole("button", { name: /session/ }),
    ).not.toBeInTheDocument();

    const toggle = within(row("Dune")).getByRole("button", {
      name: "2 sessions",
    });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    const sessions = await screen.findByRole("list", {
      name: "Sessions of Dune",
    });
    // 01:30 UTC on March 2 is still March 1 in Chicago.
    expect(sessions).toHaveTextContent(
      "Mar 1, 7:30 PM to 8:30 PM · 1h 0m · 0% to 20% · Web reader",
    );
    expect(sessions).toHaveTextContent("Kobo (estimated)");

    await user.click(
      within(sessions).getByRole("button", {
        name: "Delete session of Dune on Mar 5, 8:00 AM to 8:45 AM",
      }),
    );
    const confirm = await screen.findByRole("dialog", {
      name: "Delete this reading session?",
    });
    await user.click(within(confirm).getByRole("button", { name: "Delete" }));
    await waitFor(() => {
      const del = fetchMock.mock.calls.find(
        ([input, init]) =>
          init?.method === "DELETE" &&
          String(input).startsWith("/api/reading/sessions/"),
      );
      expect(del?.[0]).toBe("/api/reading/sessions/12");
    });
  });

  it("says when nothing matches the filters", async () => {
    renderPage({ entries: [], initialEntry: "/reading-history?removed=true" });

    expect(
      await screen.findByText("No Read-throughs match these filters."),
    ).toBeVisible();
  });
});
