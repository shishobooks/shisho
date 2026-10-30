import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";
import type { BookReadingResponse } from "@/types";

import AddToListPopover from "./AddToListPopover";

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

const stubRequests = (initial: BookReadingResponse) =>
  vi.spyOn(API, "request").mockImplementation(async (method, path) => {
    if (path === "/lists") return { items: [], total: 0 };
    if (path === "/reading/books/7/to-read") {
      return method === "POST"
        ? reading({ status: "to_read", on_to_read: true, to_read_place: 1 })
        : reading();
    }
    if (path === "/reading/books/7") return initial;
    return [];
  });

const renderPopover = () =>
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <AddToListPopover bookId={7} onOpenChange={vi.fn()} open />
    </QueryClientProvider>,
  );

const toReadItem = () =>
  screen.findByRole("menuitemcheckbox", { name: /To Read/ });

describe("AddToListPopover To Read", () => {
  beforeEach(() => setAuth({ permissions: ["books:read"] }));
  afterEach(() => vi.restoreAllMocks());

  it("offers To Read above the lists, even with no lists", async () => {
    stubRequests(reading());
    renderPopover();

    const item = await toReadItem();
    expect(item).toHaveAttribute("aria-checked", "false");
    expect(screen.getByText("No lists yet")).toBeInTheDocument();
  });

  it("adds the book to To Read", async () => {
    const request = stubRequests(reading());
    renderPopover();

    await userEvent
      .setup({ advanceTimers: vi.advanceTimersByTime })
      .click(await toReadItem());

    expect(request).toHaveBeenCalledWith(
      "POST",
      "/reading/books/7/to-read",
      null,
      null,
    );
    await waitFor(async () =>
      expect(await toReadItem()).toHaveAttribute("aria-checked", "true"),
    );
  });

  it("removes a queued book from To Read", async () => {
    const request = stubRequests(
      reading({ status: "to_read", on_to_read: true, to_read_place: 2 }),
    );
    renderPopover();

    const item = await toReadItem();
    expect(item).toHaveAttribute("aria-checked", "true");
    await userEvent
      .setup({ advanceTimers: vi.advanceTimersByTime })
      .click(item);

    expect(request).toHaveBeenCalledWith(
      "DELETE",
      "/reading/books/7/to-read",
      null,
      null,
    );
  });

  it("can't queue a book being read, and says why", async () => {
    const request = stubRequests(reading({ status: "currently_reading" }));
    renderPopover();

    const item = await toReadItem();
    expect(item).toHaveAttribute("aria-disabled", "true");
    expect(item).toHaveAccessibleDescription("You're reading this book.");
    await userEvent
      .setup({ advanceTimers: vi.advanceTimersByTime })
      .click(item);
    expect(request).not.toHaveBeenCalledWith(
      "POST",
      "/reading/books/7/to-read",
      null,
      null,
    );
  });

  it("is disabled in Demo Mode, with the demo's explanation", async () => {
    setAuth({ permissions: ["books:read"], demoMode: true });
    stubRequests(reading());
    renderPopover();

    const item = await toReadItem();
    expect(item).toHaveAttribute("aria-disabled", "true");
    expect(item).toHaveAccessibleDescription(
      "This action is unavailable in the demo.",
    );
  });
});
