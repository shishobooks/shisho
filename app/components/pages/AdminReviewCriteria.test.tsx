import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import AdminReviewCriteria from "./AdminReviewCriteria";
import { humanizeField } from "./review-criteria-utils";

beforeAll(() => {
  // @ts-expect-error - global defined by Vite
  globalThis.__APP_VERSION__ = "test";
});

// Mock useUnsavedChanges (uses react-router's useBlocker which requires a data router).
vi.mock("@/hooks/useUnsavedChanges", () => ({
  useUnsavedChanges: () => ({
    cancelNavigation: vi.fn(),
    proceedNavigation: vi.fn(),
    showBlockerDialog: false,
  }),
}));

// Configurable mock data so individual tests can override override_count
const mockCriteriaData = {
  book_fields: ["authors", "cover"],
  audio_fields: ["narrators"],
  universal_candidates: ["authors", "cover", "description"],
  audio_candidates: ["narrators", "release_date"],
  override_count: 0,
  main_file_count: 10,
};

const mockUpdateMutateAsync = vi.fn();
const mockCreateJobMutateAsync = vi.fn();

vi.mock("@/hooks/queries/review", () => ({
  useReviewCriteria: () => ({
    isLoading: false,
    isSuccess: true,
    isError: false,
    data: mockCriteriaData,
  }),
  useUpdateReviewCriteria: () => ({
    mutateAsync: mockUpdateMutateAsync,
    isPending: false,
  }),
}));

vi.mock("@/hooks/queries/jobs", () => ({
  useCreateJob: () => ({
    mutateAsync: mockCreateJobMutateAsync,
    isPending: false,
  }),
}));

const createUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime, delay: null });

function wrap(ui: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("humanizeField", () => {
  it("capitalizes single word", () => {
    expect(humanizeField("cover")).toBe("Cover");
  });

  it("converts snake_case to sentence case", () => {
    expect(humanizeField("release_date")).toBe("Release date");
  });

  it("handles multi-word snake_case", () => {
    expect(humanizeField("book_genres")).toBe("Book genres");
  });
});

describe("AdminReviewCriteria", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCriteriaData.override_count = 0;
  });

  it("renders checked checkboxes for current book_fields", () => {
    mockCriteriaData.override_count = 0;
    wrap(<AdminReviewCriteria />);

    // authors and cover are in book_fields → checked
    expect(screen.getByRole("checkbox", { name: "Authors" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Cover" })).toBeChecked();
    // description is not in book_fields → unchecked
    expect(
      screen.getByRole("checkbox", { name: "Description" }),
    ).not.toBeChecked();
  });

  it("renders checked checkbox for current audio_fields", () => {
    mockCriteriaData.override_count = 0;
    wrap(<AdminReviewCriteria />);

    expect(screen.getByRole("checkbox", { name: "Narrators" })).toBeChecked();
    expect(
      screen.getByRole("checkbox", { name: "Release date" }),
    ).not.toBeChecked();
  });

  it("Save button is disabled when no changes", () => {
    mockCriteriaData.override_count = 0;
    wrap(<AdminReviewCriteria />);

    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("Save button enables after toggling a checkbox", async () => {
    const user = createUser();
    mockCriteriaData.override_count = 0;
    wrap(<AdminReviewCriteria />);

    await user.click(screen.getByRole("checkbox", { name: "Description" }));

    expect(screen.getByRole("button", { name: "Save" })).not.toBeDisabled();
  });

  it("saves with updated arrays and clear_overrides:false when no overrides exist", async () => {
    const user = createUser();
    mockCriteriaData.override_count = 0;
    mockUpdateMutateAsync.mockResolvedValueOnce(undefined);

    wrap(<AdminReviewCriteria />);

    // Toggle description on
    await user.click(screen.getByRole("checkbox", { name: "Description" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockUpdateMutateAsync).toHaveBeenCalledWith({
        book_fields: ["authors", "cover", "description"],
        audio_fields: ["narrators"],
        clear_overrides: false,
      });
    });
  });

  it("opens confirmation dialog before saving when overrides exist", async () => {
    const user = createUser();
    mockCriteriaData.override_count = 3;

    wrap(<AdminReviewCriteria />);

    // Toggle description to enable the Save button
    await user.click(screen.getByRole("checkbox", { name: "Description" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    // Dialog should appear with override count info
    await waitFor(() => {
      expect(screen.getByText("Recompute review state?")).toBeInTheDocument();
    });
    expect(
      screen.getByText(/3 reviewed-overrides set out of 10/),
    ).toBeInTheDocument();

    // mutation should NOT have fired yet
    expect(mockUpdateMutateAsync).not.toHaveBeenCalled();
  });

  it("passes clear_overrides:true when checkbox is checked in confirmation dialog", async () => {
    const user = createUser();
    mockCriteriaData.override_count = 3;
    mockUpdateMutateAsync.mockResolvedValueOnce(undefined);

    wrap(<AdminReviewCriteria />);

    // Toggle a field to make Save enabled, then click Save
    await user.click(screen.getByRole("checkbox", { name: "Description" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    // Wait for dialog
    await waitFor(() => {
      expect(screen.getByText("Recompute review state?")).toBeInTheDocument();
    });

    // Check the "Also clear manual overrides" checkbox
    const clearCheckbox = screen.getByRole("checkbox", {
      name: "Also clear manual overrides",
    });
    await user.click(clearCheckbox);

    // Confirm
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => {
      expect(mockUpdateMutateAsync).toHaveBeenCalledWith({
        book_fields: ["authors", "cover", "description"],
        audio_fields: ["narrators"],
        clear_overrides: true,
      });
    });
  });

  it("Recompute now button queues a recompute_review job with clear_overrides:false", async () => {
    const user = createUser();
    mockCriteriaData.override_count = 0;
    mockCreateJobMutateAsync.mockResolvedValueOnce({ id: 42 });

    wrap(<AdminReviewCriteria />);

    await user.click(screen.getByRole("button", { name: "Recompute now" }));

    await waitFor(() => {
      expect(mockCreateJobMutateAsync).toHaveBeenCalledWith({
        payload: {
          type: "recompute_review",
          data: { clear_overrides: false },
        },
      });
    });
  });

  it("Recompute now button opens confirmation dialog when overrides exist", async () => {
    const user = createUser();
    mockCriteriaData.override_count = 5;

    wrap(<AdminReviewCriteria />);

    await user.click(screen.getByRole("button", { name: "Recompute now" }));

    await waitFor(() => {
      expect(screen.getByText("Recompute review state?")).toBeInTheDocument();
    });
    expect(
      screen.getByText(/5 reviewed-overrides set out of 10/),
    ).toBeInTheDocument();

    // job should NOT have fired yet
    expect(mockCreateJobMutateAsync).not.toHaveBeenCalled();
  });
});
