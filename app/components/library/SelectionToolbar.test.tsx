import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeAll, describe, expect, it, vi } from "vitest";

import { SelectionToolbar } from "./SelectionToolbar";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

// ---- hook mocks ----

const mockExitSelectionMode = vi.fn();
const mockClearSelection = vi.fn();

vi.mock("@/hooks/useBulkSelection", () => ({
  useBulkSelection: () => ({
    selectedBookIds: [1, 2, 3],
    exitSelectionMode: mockExitSelectionMode,
    clearSelection: mockClearSelection,
  }),
}));

vi.mock("@/hooks/useBulkDownload", () => ({
  useBulkDownload: () => ({ startDownload: vi.fn() }),
}));

vi.mock("@/hooks/queries/books", () => ({
  useBooks: () => ({ data: { books: [] }, isLoading: false }),
  useDeleteBooks: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

vi.mock("@/hooks/queries/jobs", () => ({
  useCreateJob: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

vi.mock("@/hooks/queries/lists", () => ({
  useListLists: () => ({ data: { lists: [] }, isLoading: false }),
  useAddBooksToList: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useCreateList: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

const mockBulkSetReviewMutateAsync = vi.fn();

vi.mock("@/hooks/queries/review", () => ({
  useBulkSetReview: () => ({
    mutateAsync: mockBulkSetReviewMutateAsync,
    isPending: false,
  }),
}));

// ---- helpers ----

const createUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime, delay: null });

function wrap(ui: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>
  );
}

// ---- tests ----

describe("SelectionToolbar — More popover bulk review actions", () => {
  it("renders the More button", () => {
    render(wrap(<SelectionToolbar />));
    expect(screen.getByRole("button", { name: /more/i })).toBeInTheDocument();
  });

  it("opens the More popover when clicked and shows both actions", async () => {
    const user = createUser();
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: /more/i }));

    expect(
      screen.getByRole("button", { name: /mark reviewed/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /mark needs review/i }),
    ).toBeInTheDocument();
  });

  it("calls useBulkSetReview with reviewed override and exits selection mode", async () => {
    const user = createUser();
    mockBulkSetReviewMutateAsync.mockResolvedValueOnce(undefined);
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: /more/i }));
    await user.click(screen.getByRole("button", { name: /mark reviewed/i }));

    expect(mockBulkSetReviewMutateAsync).toHaveBeenCalledWith({
      bookIds: [1, 2, 3],
      override: "reviewed",
    });
    expect(mockExitSelectionMode).toHaveBeenCalled();
  });

  it("calls useBulkSetReview with unreviewed override and exits selection mode", async () => {
    const user = createUser();
    mockBulkSetReviewMutateAsync.mockResolvedValueOnce(undefined);
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: /more/i }));
    await user.click(
      screen.getByRole("button", { name: /mark needs review/i }),
    );

    expect(mockBulkSetReviewMutateAsync).toHaveBeenCalledWith({
      bookIds: [1, 2, 3],
      override: "unreviewed",
    });
    expect(mockExitSelectionMode).toHaveBeenCalled();
  });
});
