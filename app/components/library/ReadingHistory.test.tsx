import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type {
  BookReadingResponse,
  ListReadThroughSessionsResponse,
  ReadThrough,
} from "@/types";

import ReadingHistory from "./ReadingHistory";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const readThrough = (id: number, end_day: string): ReadThrough => ({
  id,
  created_at: "2026-03-01T00:00:00Z",
  updated_at: "2026-03-01T00:00:00Z",
  book_id: 7,
  book_title: null,
  book_authors: null,
  begin_day: "2026-03-01",
  end_day,
  outcome: "finished",
  paused: false,
});

const reading: BookReadingResponse = {
  book_id: 7,
  status: "read",
  rating: null,
  on_to_read: false,
  read_throughs: [readThrough(2, "2026-05-02"), readThrough(1, "2026-03-09")],
  session_counts: { 1: 1, 2: 0 },
  positions: [],
};

const sessions: ListReadThroughSessionsResponse = {
  time_zone: "UTC",
  items: [
    {
      id: 31,
      started_at: "2026-03-02T19:00:00Z",
      ended_at: "2026-03-02T20:00:00Z",
      seconds: 3600,
      start_percentage: 0,
      end_percentage: 0.4,
      kind: "listening",
      source: "web_reader",
      measured: true,
    },
  ],
};

describe("ReadingHistory on a book's page", () => {
  beforeEach(() => setAuth({ permissions: ["books:read"] }));
  afterEach(() => vi.restoreAllMocks());

  it("lists a read's sessions and deletes one after confirming", async () => {
    const request = vi
      .spyOn(API, "request")
      .mockImplementation(async (method, path) => {
        if (path === "/reading/books/7") return reading;
        if (path === "/reading/read-throughs/1/sessions") return sessions;
        if (method === "DELETE") return undefined;
        return {};
      });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <ReadingHistory bookId={7} title="Dune" />
      </QueryClientProvider>,
    );

    const toggle = await screen.findByRole("button", { name: "1 session" });
    // Only the read with sessions offers them.
    expect(screen.getAllByRole("button", { name: /session/ })).toHaveLength(1);
    await user.click(toggle);
    const list = await screen.findByRole("list", { name: "Sessions of Dune" });
    expect(list).toHaveTextContent(
      "Mar 2, 7:00 PM to 8:00 PM · 1h 0m · 0% to 40% · Shisho web player",
    );

    await user.click(
      within(list).getByRole("button", { name: /^Delete session of Dune/ }),
    );
    const confirm = await screen.findByRole("dialog", {
      name: "Delete this reading session?",
    });
    await user.click(within(confirm).getByRole("button", { name: "Delete" }));
    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "DELETE",
        "/reading/sessions/31",
        null,
        null,
      ),
    );
  });
});
