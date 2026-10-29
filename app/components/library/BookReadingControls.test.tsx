import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { BookReadingResponse, ReadingStatus, ReadThrough } from "@/types";

import BookReadingControls from "./BookReadingControls";
import ReadingHistory from "./ReadingHistory";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const reading = (
  overrides: Partial<BookReadingResponse> = {},
): BookReadingResponse => ({
  book_id: 7,
  status: null,
  rating: null,
  on_to_read: false,
  read_throughs: [],
  ...overrides,
});

const readThrough = (overrides: Partial<ReadThrough>): ReadThrough => ({
  id: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  book_id: 7,
  begin_day: undefined,
  end_day: undefined,
  outcome: "finished",
  paused: false,
  ...overrides,
});

const renderControls = (initial: BookReadingResponse) => {
  const request = vi.spyOn(API, "request").mockResolvedValue(initial);
  render(
    <QueryClientProvider client={new QueryClient()}>
      <BookReadingControls bookId={7} />
      <ReadingHistory bookId={7} />
    </QueryClientProvider>,
  );
  return request;
};

const user = () => userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

const openStatusMenu = async (status: string) => {
  await user().click(
    await screen.findByRole("button", { name: `Reading status: ${status}` }),
  );
};

const menuItems = () =>
  screen.getAllByRole("menuitem").map((item) => item.textContent);

describe("BookReadingControls", () => {
  beforeEach(() => setAuth({ permissions: ["books:read"] }));
  afterEach(() => vi.restoreAllMocks());

  it.each<[ReadingStatus | null, string, string[]]>([
    [
      null,
      "Track reading",
      ["Start reading", "Mark as read", "Add to To Read"],
    ],
    [
      "to_read",
      "To Read",
      ["Start reading", "Mark as read", "Remove from To Read"],
    ],
    [
      "currently_reading",
      "Currently Reading",
      ["Pause", "Mark as read", "Mark as Did Not Finish"],
    ],
    ["paused", "Paused", ["Resume", "Mark as read", "Mark as Did Not Finish"]],
    ["read", "Read", ["Read again", "Mark as read", "Add to To Read"]],
    [
      "did_not_finish",
      "Did Not Finish",
      ["Read again", "Mark as read", "Add to To Read"],
    ],
  ])(
    "offers only the valid transitions from %s",
    async (status, label, items) => {
      renderControls(reading({ status, on_to_read: status === "to_read" }));
      await openStatusMenu(label);
      expect(menuItems()).toEqual(items);
    },
  );

  it("adds a read book to To Read to plan a reread", async () => {
    const request = renderControls(reading({ status: "read" }));
    await openStatusMenu("Read");
    request.mockResolvedValue(reading({ status: "to_read", on_to_read: true }));
    await user().click(
      screen.getByRole("menuitem", { name: "Add to To Read" }),
    );

    expect(request).toHaveBeenCalledWith(
      "POST",
      "/reading/books/7/to-read",
      null,
      null,
    );
    expect(
      await screen.findByRole("button", { name: "Reading status: To Read" }),
    ).toBeInTheDocument();
  });

  it("removes a book from To Read", async () => {
    const request = renderControls(
      reading({ status: "to_read", on_to_read: true }),
    );
    await openStatusMenu("To Read");
    request.mockResolvedValue(reading());
    await user().click(
      screen.getByRole("menuitem", { name: "Remove from To Read" }),
    );

    expect(request).toHaveBeenCalledWith(
      "DELETE",
      "/reading/books/7/to-read",
      null,
      null,
    );
    expect(
      await screen.findByRole("button", {
        name: "Reading status: Track reading",
      }),
    ).toBeInTheDocument();
  });

  it("marks a book read with the browser's local day", async () => {
    vi.setSystemTime(new Date(2026, 9, 28, 23, 30));
    const request = renderControls(reading());
    await openStatusMenu("Track reading");
    request.mockResolvedValue(
      reading({
        status: "read",
        read_throughs: [readThrough({ end_day: "2026-10-28" })],
      }),
    );
    await user().click(screen.getByRole("menuitem", { name: "Mark as read" }));

    expect(request).toHaveBeenCalledWith(
      "POST",
      "/reading/books/7/finish",
      { today: "2026-10-28" },
      null,
    );
    expect(
      await screen.findByRole("button", { name: "Reading status: Read" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Read once, last finished Oct 2026"),
    ).toBeInTheDocument();
  });

  it("clears a rating", async () => {
    const request = renderControls(reading({ rating: 3.75 }));
    const slider = await screen.findByRole("slider", { name: "Rating" });
    expect(slider).toHaveAttribute("aria-valuetext", "3.75 stars");

    request.mockResolvedValue(reading());
    await user().click(screen.getByRole("button", { name: "Clear rating" }));
    expect(request).toHaveBeenCalledWith(
      "PUT",
      "/reading/books/7/rating",
      { rating: null },
      null,
    );
    await waitFor(() =>
      expect(screen.getByRole("slider", { name: "Rating" })).toHaveAttribute(
        "aria-valuetext",
        "Not rated",
      ),
    );
  });

  it("rates in quarter stars from the keyboard", async () => {
    const request = renderControls(reading({ rating: 3.5 }));
    const slider = await screen.findByRole("slider", { name: "Rating" });
    request.mockResolvedValue(reading({ rating: 3.75 }));
    slider.focus();
    await user().keyboard("{ArrowRight}");
    expect(request).toHaveBeenCalledWith(
      "PUT",
      "/reading/books/7/rating",
      { rating: 3.75 },
      null,
    );
  });

  it("renders disabled in Demo Mode, with the demo's explanation", async () => {
    setAuth({ permissions: ["books:read"], demoMode: true });
    renderControls(
      reading({ status: "read", rating: 4, read_throughs: [readThrough({})] }),
    );
    expect(
      await screen.findByRole("button", { name: "Reading status: Read" }),
    ).toBeDisabled();
    expect(screen.getByRole("slider", { name: "Rating" })).toHaveAttribute(
      "data-disabled",
    );
    expect(
      screen.getByText("This action is unavailable in the demo."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Clear rating" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Edit dates" })).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Delete Read-through" }),
    ).toBeDisabled();
  });
});

describe("ReadingHistory", () => {
  beforeEach(() => setAuth({ permissions: ["books:read"] }));
  afterEach(() => vi.restoreAllMocks());

  it("shows nothing for an unread book", async () => {
    const request = renderControls(reading());
    await screen.findByRole("button", {
      name: "Reading status: Track reading",
    });
    expect(request).toHaveBeenCalled();
    expect(screen.queryByText("Reading history")).not.toBeInTheDocument();
  });

  it("lists each Read-through with its dates and outcome", async () => {
    renderControls(
      reading({
        status: "did_not_finish",
        read_throughs: [
          readThrough({
            id: 2,
            outcome: "abandoned",
            begin_day: "2026-06-01",
            end_day: "2026-06-10",
          }),
          readThrough({ id: 1, end_day: "2024-03-02" }),
        ],
      }),
    );
    expect(await screen.findByText("Reading history")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Read once, last finished Mar 2024 · Didn't finish once",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("Jun 1, 2026 to Jun 10, 2026")).toBeInTheDocument();
    expect(
      screen.getByText("Ended Mar 2, 2024, start date unknown"),
    ).toBeInTheDocument();
  });

  it("clears a start date", async () => {
    const request = renderControls(
      reading({
        status: "read",
        read_throughs: [
          readThrough({ begin_day: "2026-03-01", end_day: "2026-03-20" }),
        ],
      }),
    );
    await user().click(
      await screen.findByRole("button", { name: "Edit dates" }),
    );
    const started = screen.getByLabelText("Started");
    expect(started).toHaveTextContent("Mar 1, 2026");
    await user().click(started);
    await user().click(await screen.findByRole("button", { name: "Clear" }));
    expect(screen.getByLabelText("Started")).toHaveTextContent("Unknown");
    request.mockResolvedValue(
      reading({
        status: "read",
        read_throughs: [readThrough({ end_day: "2026-03-20" })],
      }),
    );
    await user().click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "PATCH",
        "/reading/read-throughs/1",
        { begin_day: null, end_day: "2026-03-20" },
        null,
      ),
    );
    expect(
      await screen.findByText("Ended Mar 20, 2026, start date unknown"),
    ).toBeInTheDocument();
  });

  it("deletes a Read-through after confirming", async () => {
    const request = renderControls(
      reading({ status: "read", read_throughs: [readThrough({ id: 5 })] }),
    );
    await user().click(
      await screen.findByRole("button", { name: "Delete Read-through" }),
    );
    request.mockResolvedValue(reading());
    await user().click(screen.getByRole("button", { name: "Delete" }));

    expect(request).toHaveBeenCalledWith(
      "DELETE",
      "/reading/read-throughs/5",
      null,
      null,
    );
    await waitFor(() =>
      expect(screen.queryByText("Reading history")).not.toBeInTheDocument(),
    );
  });

  it("picks a finish date from the calendar", async () => {
    vi.setSystemTime(new Date(2026, 2, 25, 12));
    const request = renderControls(
      reading({
        status: "read",
        read_throughs: [
          readThrough({ begin_day: "2026-03-01", end_day: "2026-03-20" }),
        ],
      }),
    );
    await user().click(
      await screen.findByRole("button", { name: "Edit dates" }),
    );
    await user().click(screen.getByLabelText("Finished"));
    // Days after today can't be picked.
    expect(
      await screen.findByRole("button", { name: /March 26th, 2026/ }),
    ).toBeDisabled();
    await user().click(
      screen.getByRole("button", { name: /March 18th, 2026/ }),
    );
    expect(screen.getByLabelText("Finished")).toHaveTextContent("Mar 18, 2026");

    request.mockResolvedValue(reading());
    await user().click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(request).toHaveBeenCalledWith(
        "PATCH",
        "/reading/read-throughs/1",
        { begin_day: "2026-03-01", end_day: "2026-03-18" },
        null,
      ),
    );
  });
});
