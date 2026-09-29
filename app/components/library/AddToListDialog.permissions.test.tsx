import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import { AddToListDialog } from "./AddToListDialog";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const lists = [
  { id: 1, name: "My Picks", book_count: 2, permission: "owner" },
  { id: 2, name: "Club Reads", book_count: 4, permission: "viewer" },
];

const stubRequests = () =>
  vi.spyOn(API, "request").mockImplementation(async (_method, path) => {
    if (path === "/lists") return { items: lists, total: lists.length };
    return [];
  });

const renderDialog = (open: boolean) => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <AddToListDialog bookId={7} onOpenChange={vi.fn()} open={open} />
    </QueryClientProvider>,
  );
};

// BookItem mounts one of these per card, so a closed dialog must stay quiet.
describe("AddToListDialog requests", () => {
  beforeEach(() => setAuth({ permissions: ["books:read"] }));

  afterEach(() => vi.restoreAllMocks());

  it("sends nothing while closed", async () => {
    const request = stubRequests();

    renderDialog(false);
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(request).not.toHaveBeenCalled();
  });

  it("hides lists the user can only view", async () => {
    stubRequests();

    renderDialog(true);

    expect(
      await screen.findByRole("menuitemcheckbox", { name: /My Picks/ }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Club Reads")).not.toBeInTheDocument();
  });

  it("loads the lists and the book's lists once open", async () => {
    const request = stubRequests();

    renderDialog(true);

    await waitFor(() => {
      const paths = request.mock.calls.map((call) => call[1]);
      expect(paths).toContain("/lists");
      expect(paths).toContain("/books/7/lists");
    });
  });
});
