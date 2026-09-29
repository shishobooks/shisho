import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import AddToListPopover from "./AddToListPopover";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const lists = [
  { id: 1, name: "My Picks", book_count: 2, permission: "owner" },
  { id: 2, name: "Club Reads", book_count: 4, permission: "viewer" },
  { id: 3, name: "Team Shelf", book_count: 1, permission: "editor" },
];

const stubRequests = (items = lists) =>
  vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
    if (path === "/lists") return { items, total: items.length };
    return [];
  });

const renderPopover = (open: boolean) => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <AddToListPopover bookId={7} onOpenChange={vi.fn()} open={open} />
    </QueryClientProvider>,
  );
};

const flush = () =>
  act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });

describe("AddToListPopover", () => {
  beforeEach(() => setAuth({ permissions: ["books:read"] }));

  afterEach(() => vi.restoreAllMocks());

  it("hides lists the user can only view", async () => {
    stubRequests();

    renderPopover(true);

    expect(
      await screen.findByRole("menuitemcheckbox", { name: "My Picks" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("menuitemcheckbox", { name: "Team Shelf" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Club Reads")).not.toBeInTheDocument();
  });

  it("says no list is editable when every list is view-only", async () => {
    stubRequests([lists[1]]);

    renderPopover(true);

    expect(
      await screen.findByText("No lists you can edit"),
    ).toBeInTheDocument();
    expect(screen.queryByText("No lists yet")).not.toBeInTheDocument();
  });

  it("sends no request while closed", async () => {
    const request = stubRequests();

    renderPopover(false);
    await flush();

    expect(request).not.toHaveBeenCalled();
  });
});
