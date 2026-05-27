import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

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

const mockStartDownload = vi.fn();

vi.mock("@/hooks/useBulkDownload", () => ({
  useBulkDownload: () => ({ startDownload: mockStartDownload }),
}));

const mockBooks = [
  {
    id: 1,
    title: "Book One",
    files: [
      {
        id: 101,
        file_type: "epub",
        file_role: "main",
        filesize_bytes: 1000,
      },
    ],
  },
  {
    id: 2,
    title: "Book Two",
    files: [
      {
        id: 201,
        file_type: "epub",
        file_role: "main",
        filesize_bytes: 2000,
      },
      {
        id: 202,
        file_type: "m4b",
        file_role: "main",
        filesize_bytes: 5000,
      },
    ],
  },
  {
    id: 3,
    title: "Book Three",
    files: [
      {
        id: 301,
        file_type: "cbz",
        file_role: "main",
        filesize_bytes: 3000,
      },
      {
        id: 302,
        file_type: "pdf",
        file_role: "supplement",
        filesize_bytes: 500,
      },
    ],
  },
];

vi.mock("@/hooks/queries/books", () => ({
  useBooks: () => ({
    data: { items: mockBooks, total: 3 },
    isLoading: false,
  }),
  useDeleteBooks: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

const mockCreateJobMutateAsync = vi.fn();

vi.mock("@/hooks/queries/jobs", () => ({
  useCreateJob: () => ({
    mutateAsync: mockCreateJobMutateAsync,
    isPending: false,
  }),
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

describe("SelectionToolbar — Download file-type selection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows file-type checkboxes in the download popover", async () => {
    const user = createUser();
    render(wrap(<SelectionToolbar />));

    // The desktop Download button should be present with size info
    const downloadBtn = screen.getByRole("button", { name: /download/i });
    await user.click(downloadBtn);

    // All four file-type labels should appear in the popover
    expect(screen.getByText("EPUB")).toBeInTheDocument();
    expect(screen.getByText("M4B")).toBeInTheDocument();
    expect(screen.getByText("CBZ")).toBeInTheDocument();
    expect(screen.getByText("PDF")).toBeInTheDocument();
  });

  it("checks available file types by default and disables unavailable ones", async () => {
    const user = createUser();
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: /download/i }));

    const checkboxes = screen.getAllByRole("checkbox");

    // EPUB (index 0), M4B (index 1), CBZ (index 2) are available main file types
    expect(checkboxes[0]).toHaveAttribute("data-state", "checked");
    expect(checkboxes[1]).toHaveAttribute("data-state", "checked");
    expect(checkboxes[2]).toHaveAttribute("data-state", "checked");
    // PDF only exists as supplement, so it should be disabled
    expect(checkboxes[3]).toBeDisabled();
  });

  it("updates file count when toggling file types", async () => {
    const user = createUser();
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: /download/i }));

    // Initially: 4 files (101 epub, 201 epub, 202 m4b, 301 cbz)
    expect(screen.getByText(/4 files/)).toBeInTheDocument();

    // Uncheck M4B
    const checkboxes = screen.getAllByRole("checkbox");
    await user.click(checkboxes[1]); // M4B

    // Now: 3 files
    expect(screen.getByText(/3 files/)).toBeInTheDocument();
  });

  it("submits only selected file types for download", async () => {
    const user = createUser();
    mockCreateJobMutateAsync.mockResolvedValueOnce({ id: 42 });
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: /download/i }));

    const checkboxes = screen.getAllByRole("checkbox");

    // Uncheck M4B and CBZ, keep only EPUB
    await user.click(checkboxes[1]); // M4B
    await user.click(checkboxes[2]); // CBZ

    // Click the Download button inside the popover (second one, first is the trigger)
    const downloadButtons = screen.getAllByRole("button", {
      name: /download/i,
    });
    // The last "Download" button is the submit button inside the popover
    await user.click(downloadButtons[downloadButtons.length - 1]);

    expect(mockCreateJobMutateAsync).toHaveBeenCalledWith({
      payload: {
        type: "bulk_download",
        data: {
          file_ids: [101, 201],
          estimated_size_bytes: 3000,
        },
      },
    });
  });

  it("excludes supplement files even when their type matches", async () => {
    const user = createUser();
    mockCreateJobMutateAsync.mockResolvedValueOnce({ id: 42 });
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: /download/i }));

    const checkboxes = screen.getAllByRole("checkbox");

    // Uncheck EPUB, M4B — keep only CBZ
    await user.click(checkboxes[0]); // EPUB
    await user.click(checkboxes[1]); // M4B

    const downloadButtons = screen.getAllByRole("button", {
      name: /download/i,
    });
    await user.click(downloadButtons[downloadButtons.length - 1]);

    // Only the main CBZ file (301), not the supplement PDF (302)
    expect(mockCreateJobMutateAsync).toHaveBeenCalledWith({
      payload: {
        type: "bulk_download",
        data: {
          file_ids: [301],
          estimated_size_bytes: 3000,
        },
      },
    });
  });
});
