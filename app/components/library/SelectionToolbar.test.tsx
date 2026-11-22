import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { toast } from "sonner";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { setAuth } from "@/testing/auth";

import { SelectionToolbar } from "./SelectionToolbar";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

// ---- hook mocks ----

const mockExitSelectionMode = vi.fn();
const mockClearSelection = vi.fn();
const selection = vi.hoisted(() => ({ ids: [1, 2, 3] }));

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

beforeEach(() => {
  setAuth({ permissions: ["books:read", "books:write"] });
  selection.ids = [1, 2, 3];
});

vi.mock("@/hooks/useBulkSelection", () => ({
  useBulkSelection: () => ({
    selectedBookIds: selection.ids,
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
    reading_status: "currently_reading",
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
    reading_status: "paused",
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
    reading_status: "read",
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

const mockUseBooks = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/queries/books", () => ({
  useBooks: (...args: unknown[]) => {
    mockUseBooks(...args);
    return { data: { items: mockBooks, total: 3 }, isLoading: false };
  },
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
  useListLists: () => ({
    data: {
      items: [
        { id: 9, name: "My queue", permission: "owner" },
        { id: 10, name: "Shared read-only", permission: "viewer" },
      ],
    },
    isLoading: false,
  }),
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

const mockBulkReadingMutateAsync = vi.fn();

vi.mock("@/hooks/queries/reading", () => ({
  useBulkReadingChange: () => ({
    mutateAsync: mockBulkReadingMutateAsync,
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

describe("SelectionToolbar bulk review actions in the More popover", () => {
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

  it("groups thousands in the count it reports", async () => {
    const user = createUser();
    const success = vi.spyOn(toast, "success");
    selection.ids = Array.from({ length: 1200 }, (_, i) => i + 1);
    mockBulkSetReviewMutateAsync.mockResolvedValueOnce(undefined);
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: /more/i }));
    await user.click(screen.getByRole("button", { name: /mark reviewed/i }));

    expect(success).toHaveBeenCalledWith("Marked 1,200 books as reviewed");
    success.mockRestore();
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

describe("SelectionToolbar exit button", () => {
  it("is named and exits selection mode", async () => {
    const user = createUser();
    mockExitSelectionMode.mockClear();
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: "Exit selection" }));

    expect(mockExitSelectionMode).toHaveBeenCalledOnce();
  });
});

const fileTypeBox = (label: string) =>
  screen.getByRole("checkbox", { name: label });

describe("SelectionToolbar download file-type selection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows file-type checkboxes in the download popover", async () => {
    const user = createUser();
    render(wrap(<SelectionToolbar />));

    // The desktop Download button should be present with size info
    const downloadBtn = screen.getByRole("button", { name: /download/i });
    await user.click(downloadBtn);

    // Every file-type label should appear in the popover
    for (const label of ["EPUB", "AZW3", "MOBI", "M4B", "CBZ", "PDF"]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
  });

  it("checks available file types by default and disables unavailable ones", async () => {
    const user = createUser();
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: /download/i }));

    // EPUB, M4B, and CBZ are available main file types
    expect(fileTypeBox("EPUB")).toHaveAttribute("data-state", "checked");
    expect(fileTypeBox("M4B")).toHaveAttribute("data-state", "checked");
    expect(fileTypeBox("CBZ")).toHaveAttribute("data-state", "checked");
    // PDF only exists as supplement, so it should be disabled
    expect(fileTypeBox("PDF")).toBeDisabled();
    // No selected book has an AZW3 or MOBI file
    expect(fileTypeBox("AZW3")).toBeDisabled();
    expect(fileTypeBox("MOBI")).toBeDisabled();
  });

  it("updates file count when toggling file types", async () => {
    const user = createUser();
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: /download/i }));

    // Initially: 4 files (101 epub, 201 epub, 202 m4b, 301 cbz)
    expect(screen.getByText(/4 files/)).toBeInTheDocument();

    // Uncheck M4B
    await user.click(fileTypeBox("M4B"));

    // Now: 3 files
    expect(screen.getByText(/3 files/)).toBeInTheDocument();
  });

  it("disables the popover Download button when no file types are checked", async () => {
    const user = createUser();
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: /download/i }));
    const downloadButtons = screen.getAllByRole("button", {
      name: /download/i,
    });
    const submit = downloadButtons[downloadButtons.length - 1];
    expect(submit).toBeEnabled();

    // Uncheck EPUB, M4B and CBZ, the only available types.
    await user.click(fileTypeBox("EPUB"));
    await user.click(fileTypeBox("M4B"));
    await user.click(fileTypeBox("CBZ"));
    expect(submit).toBeDisabled();

    await user.click(fileTypeBox("CBZ"));
    expect(submit).toBeEnabled();
  });

  it("submits only selected file types for download", async () => {
    const user = createUser();
    mockCreateJobMutateAsync.mockResolvedValueOnce({ id: 42 });
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: /download/i }));

    // Uncheck M4B and CBZ, keep only EPUB
    await user.click(fileTypeBox("M4B"));
    await user.click(fileTypeBox("CBZ"));

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

    // Uncheck EPUB and M4B to keep only CBZ
    await user.click(fileTypeBox("EPUB"));
    await user.click(fileTypeBox("M4B"));

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

describe("SelectionToolbar read-only user", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("keeps Add and Download but hides Merge, Delete, and review actions", async () => {
    setAuth({ permissions: ["books:read"] });
    const user = createUser();
    render(wrap(<SelectionToolbar library={{ id: 1 } as never} />));

    expect(screen.getByRole("button", { name: /^add$/i })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /download/i }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /merge/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /delete/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /^more$/i }),
    ).not.toBeInTheDocument();

    // The mobile Actions popover follows the same rule.
    await user.click(screen.getByRole("button", { name: /^actions$/i }));
    expect(
      screen.queryByRole("button", { name: /mark reviewed/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /mark needs review/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /delete/i }),
    ).not.toBeInTheDocument();
  });

  it("still offers the lists the user can edit", async () => {
    setAuth({ permissions: ["books:read"] });
    const user = createUser();
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: /^add$/i }));
    expect(
      screen.getByRole("button", { name: "My queue" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Shared read-only" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /create new list/i }),
    ).toBeInTheDocument();
  });
});

describe("SelectionToolbar reading status actions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("marks the selected books read and reports what it skipped", async () => {
    const user = createUser();
    const success = vi.spyOn(toast, "success");
    mockBulkReadingMutateAsync.mockResolvedValueOnce({
      changed: 2,
      skipped: 1,
    });
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: "Reading" }));
    await user.click(screen.getByRole("button", { name: "Mark as read" }));

    expect(mockBulkReadingMutateAsync).toHaveBeenCalledWith({
      action: "finish",
      bookIds: [1, 2, 3],
    });
    expect(success).toHaveBeenCalledWith("Marked 2 books as read, 1 skipped");
    expect(mockExitSelectionMode).toHaveBeenCalled();
    success.mockRestore();
  });

  it("leaves out the skipped count when nothing was skipped", async () => {
    const user = createUser();
    const success = vi.spyOn(toast, "success");
    mockBulkReadingMutateAsync.mockResolvedValueOnce({
      changed: 1,
      skipped: 0,
    });
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: "Reading" }));
    await user.click(screen.getByRole("button", { name: "Mark as read" }));

    expect(success).toHaveBeenCalledWith("Marked 1 book as read");
    success.mockRestore();
  });

  it("confirms marking not started, naming how many reads it deletes", async () => {
    const user = createUser();
    const success = vi.spyOn(toast, "success");
    mockBulkReadingMutateAsync.mockResolvedValueOnce({
      changed: 2,
      skipped: 1,
    });
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: "Reading" }));
    await user.click(
      screen.getByRole("button", { name: "Mark as not started" }),
    );
    expect(mockBulkReadingMutateAsync).not.toHaveBeenCalled();

    const dialog = screen.getByRole("dialog");
    // Books One and Two are being read; Book Three is already read.
    expect(dialog).toHaveTextContent(
      "The Read-throughs underway on 2 books are deleted",
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Mark as not started" }),
    );

    expect(mockBulkReadingMutateAsync).toHaveBeenCalledWith({
      action: "not-started",
      bookIds: [1, 2, 3],
    });
    expect(success).toHaveBeenCalledWith(
      "Marked 2 books as not started, 1 skipped",
    );
    expect(mockExitSelectionMode).toHaveBeenCalled();
    success.mockRestore();
  });

  it("says nothing is being read instead of confirming, when none is", async () => {
    const user = createUser();
    const info = vi.spyOn(toast, "info");
    selection.ids = [3];
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: "Reading" }));
    await user.click(
      screen.getByRole("button", { name: "Mark as not started" }),
    );

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(info).toHaveBeenCalledWith(
      "None of the selected books is being read.",
    );
    expect(mockBulkReadingMutateAsync).not.toHaveBeenCalled();
    info.mockRestore();
  });

  it("confirms marking Reference, naming how many reads it deletes", async () => {
    const user = createUser();
    const success = vi.spyOn(toast, "success");
    mockBulkReadingMutateAsync.mockResolvedValueOnce({
      changed: 3,
      skipped: 0,
    });
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: "Reading" }));
    await user.click(screen.getByRole("button", { name: "Mark as Reference" }));
    expect(mockBulkReadingMutateAsync).not.toHaveBeenCalled();

    const dialog = screen.getByRole("dialog");
    // Books One and Two are being read; Book Three is already read.
    expect(dialog).toHaveTextContent(
      "The current Read-throughs of 2 books are deleted from your reading history, and this can't be undone. The reading time recorded during them still counts",
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Mark as Reference" }),
    );

    expect(mockBulkReadingMutateAsync).toHaveBeenCalledWith({
      action: "reference",
      bookIds: [1, 2, 3],
    });
    expect(success).toHaveBeenCalledWith("Marked 3 books as Reference");
    expect(mockExitSelectionMode).toHaveBeenCalled();
    success.mockRestore();
  });

  it("marks Reference without confirming when none is being read", async () => {
    const user = createUser();
    mockBulkReadingMutateAsync.mockResolvedValueOnce({
      changed: 1,
      skipped: 0,
    });
    selection.ids = [3];
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: "Reading" }));
    await user.click(screen.getByRole("button", { name: "Mark as Reference" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(mockBulkReadingMutateAsync).toHaveBeenCalledWith({
      action: "reference",
      bookIds: [3],
    });
  });

  it("confirms marking Reference when a selected book hasn't loaded", async () => {
    const user = createUser();
    // Book 99 isn't among the loaded books, so it may be being read.
    selection.ids = [3, 99];
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: "Reading" }));
    await user.click(screen.getByRole("button", { name: "Mark as Reference" }));

    expect(mockBulkReadingMutateAsync).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toHaveTextContent(
      "The current Read-through of each selected book is deleted from your reading history",
    );
  });

  it("removes the selected books from Reference", async () => {
    const user = createUser();
    const success = vi.spyOn(toast, "success");
    mockBulkReadingMutateAsync.mockResolvedValueOnce({
      changed: 1,
      skipped: 2,
    });
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: "Reading" }));
    await user.click(
      screen.getByRole("button", { name: "Remove from Reference" }),
    );

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(mockBulkReadingMutateAsync).toHaveBeenCalledWith({
      action: "remove-reference",
      bookIds: [1, 2, 3],
    });
    expect(success).toHaveBeenCalledWith(
      "Removed 1 book from Reference, 2 skipped",
    );
    success.mockRestore();
  });

  it("fetches every selected book, up to the most one page holds", () => {
    selection.ids = Array.from({ length: 30 }, (_, i) => i + 1);
    render(wrap(<SelectionToolbar />));

    expect(mockUseBooks).toHaveBeenLastCalledWith(
      expect.objectContaining({ ids: selection.ids, limit: 30 }),
      expect.anything(),
    );
  });

  it("confirms without a count when not every selected book has loaded", async () => {
    const user = createUser();
    // More selected than the three that loaded, such as on other pages.
    selection.ids = Array.from({ length: 30 }, (_, i) => i + 1);
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: "Reading" }));
    await user.click(
      screen.getByRole("button", { name: "Mark as not started" }),
    );

    expect(screen.getByRole("dialog")).toHaveTextContent(
      "The Read-through underway on each selected book is deleted",
    );
  });

  it("keeps the selection when the change fails", async () => {
    const user = createUser();
    const failure = vi.spyOn(toast, "error");
    mockBulkReadingMutateAsync.mockRejectedValueOnce(new Error("boom"));
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: "Reading" }));
    await user.click(screen.getByRole("button", { name: "Mark as read" }));

    expect(failure).toHaveBeenCalled();
    expect(mockExitSelectionMode).not.toHaveBeenCalled();
    failure.mockRestore();
  });

  it("offers both actions in the phone Actions menu", async () => {
    const user = createUser();
    mockBulkReadingMutateAsync.mockResolvedValueOnce({
      changed: 2,
      skipped: 1,
    });
    render(wrap(<SelectionToolbar />));

    await user.click(screen.getByRole("button", { name: /^actions$/i }));
    const menu = screen.getByRole("dialog");
    expect(
      within(menu).getByRole("button", { name: "Mark as not started" }),
    ).toBeInTheDocument();
    await user.click(
      within(menu).getByRole("button", { name: "Mark as read" }),
    );

    expect(mockBulkReadingMutateAsync).toHaveBeenCalledWith({
      action: "finish",
      bookIds: [1, 2, 3],
    });
  });

  it("is offered to a user who can only read books", () => {
    setAuth({ permissions: ["books:read"] });
    render(wrap(<SelectionToolbar />));

    expect(screen.getByRole("button", { name: "Reading" })).toBeInTheDocument();
  });
});
