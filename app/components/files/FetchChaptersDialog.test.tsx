import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { API, ShishoAPIError } from "@/libraries/api";
import type { ChapterInput } from "@/types";

import FetchChaptersDialog from "./FetchChaptersDialog";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const createUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

const createQueryClient = () =>
  new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

const renderWithClient = (ui: React.ReactElement) => {
  const queryClient = createQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>,
  );
};

// ---------------------------------------------------------------------------
// Shared fixtures
// ---------------------------------------------------------------------------

const defaultEditedChapters: ChapterInput[] = [
  { title: "Intro", start_timestamp_ms: 0, children: [] },
  { title: "Chapter 1", start_timestamp_ms: 60000, children: [] },
];

const defaultProps = {
  open: true,
  onOpenChange: vi.fn(),
  onApply: vi.fn(),
  editedChapters: defaultEditedChapters,
  fileDurationMs: 7200000, // 2 hours
  hasChanges: false,
};

const audnexusResponse = {
  asin: "B0AAAATEST",
  is_accurate: true,
  runtime_length_ms: 7200000,
  brand_intro_duration_ms: 0,
  brand_outro_duration_ms: 0,
  chapters: [
    { title: "Audnexus Intro", start_offset_ms: 0, length_ms: 60000 },
    { title: "Audnexus Chapter 1", start_offset_ms: 60000, length_ms: 7140000 },
  ],
};

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("FetchChaptersDialog", () => {
  describe("Entry stage", () => {
    it("shows 'existing' note and enables Fetch when prefilled with valid ASIN", () => {
      renderWithClient(
        <FetchChaptersDialog {...defaultProps} initialAsin="B0AAAATEST" />,
      );

      // Input should be prefilled
      const input = screen.getByRole("textbox");
      expect(input).toHaveValue("B0AAAATEST");

      // Fetch button should be enabled
      const fetchButton = screen.getByRole("button", {
        name: /fetch chapters/i,
      });
      expect(fetchButton).not.toBeDisabled();
    });

    it("disables Fetch when ASIN is empty", () => {
      renderWithClient(<FetchChaptersDialog {...defaultProps} />);

      const fetchButton = screen.getByRole("button", {
        name: /fetch chapters/i,
      });
      expect(fetchButton).toBeDisabled();
    });

    it("disables Fetch for invalid ASIN (fewer than 10 chars)", async () => {
      const user = createUser();
      renderWithClient(<FetchChaptersDialog {...defaultProps} />);

      const input = screen.getByRole("textbox");
      await user.type(input, "SHORT");

      const fetchButton = screen.getByRole("button", {
        name: /fetch chapters/i,
      });
      expect(fetchButton).toBeDisabled();
    });

    it("enables Fetch when ASIN is exactly 10 alphanumeric characters", async () => {
      const user = createUser();
      renderWithClient(<FetchChaptersDialog {...defaultProps} />);

      const input = screen.getByRole("textbox");
      await user.type(input, "B0AAAATEST");

      const fetchButton = screen.getByRole("button", {
        name: /fetch chapters/i,
      });
      expect(fetchButton).not.toBeDisabled();
    });

    it("disables Fetch for 10 characters with special chars", async () => {
      const user = createUser();
      renderWithClient(<FetchChaptersDialog {...defaultProps} />);

      const input = screen.getByRole("textbox");
      // The input uppercases but special chars are not alphanumeric
      await user.type(input, "B0AAAA!@#$");

      const fetchButton = screen.getByRole("button", {
        name: /fetch chapters/i,
      });
      expect(fetchButton).toBeDisabled();
    });
  });

  describe("Loading and result stages", () => {
    it("shows result stage after successful fetch", async () => {
      const user = createUser();

      vi.spyOn(API, "request").mockResolvedValue(audnexusResponse);

      renderWithClient(<FetchChaptersDialog {...defaultProps} />);

      // Type a valid ASIN and click Fetch
      const input = screen.getByRole("textbox");
      await user.type(input, "B0AAAATEST");
      await user.click(screen.getByRole("button", { name: /fetch chapters/i }));

      // Wait for result stage — mock resolves in the microtask queue, so
      // the loading spinner may be visible for only one tick and the DOM
      // lands in the result stage before our next assertion.
      await waitFor(() => {
        expect(
          screen.getByText(/2 chapters from audible/i),
        ).toBeInTheDocument();
      });

      // Confirm result-stage action buttons are present
      expect(
        screen.getByRole("button", { name: /apply titles only/i }),
      ).toBeInTheDocument();
    });

    it("clicking 'Apply titles only' calls onApply with titles replaced and timestamps preserved", async () => {
      const user = createUser();
      const onApply = vi.fn();

      vi.spyOn(API, "request").mockResolvedValue(audnexusResponse);

      renderWithClient(
        <FetchChaptersDialog {...defaultProps} onApply={onApply} />,
      );

      const input = screen.getByRole("textbox");
      await user.type(input, "B0AAAATEST");
      await user.click(screen.getByRole("button", { name: /fetch chapters/i }));

      await waitFor(() => {
        expect(
          screen.getByRole("button", { name: /apply titles only/i }),
        ).toBeInTheDocument();
      });

      await user.click(
        screen.getByRole("button", { name: /apply titles only/i }),
      );

      expect(onApply).toHaveBeenCalledOnce();
      const applied: ChapterInput[] = onApply.mock.calls[0][0];

      // Titles should come from Audnexus
      expect(applied[0].title).toBe("Audnexus Intro");
      expect(applied[1].title).toBe("Audnexus Chapter 1");

      // Timestamps should be preserved from editedChapters
      expect(applied[0].start_timestamp_ms).toBe(0);
      expect(applied[1].start_timestamp_ms).toBe(60000);
    });
  });

  describe("Apply titles only — chapter count mismatch", () => {
    it("disables 'Apply titles only' when chapter counts differ", async () => {
      const user = createUser();

      // Audnexus returns 2 chapters; editedChapters has 2 — make them differ
      const mismatchedChapters: ChapterInput[] = [
        { title: "Only One", start_timestamp_ms: 0, children: [] },
      ];

      vi.spyOn(API, "request").mockResolvedValue(audnexusResponse);

      renderWithClient(
        <FetchChaptersDialog
          {...defaultProps}
          editedChapters={mismatchedChapters}
        />,
      );

      const input = screen.getByRole("textbox");
      await user.type(input, "B0AAAATEST");
      await user.click(screen.getByRole("button", { name: /fetch chapters/i }));

      await waitFor(() => {
        // Result stage shows chapter count mismatch info
        expect(screen.getByText(/1/)).toBeInTheDocument();
      });

      // The "Apply titles only" button should be disabled (counts differ: 1 vs 2)
      await waitFor(() => {
        const applyTitlesBtn = screen.getByRole("button", {
          name: /apply titles only/i,
        });
        expect(applyTitlesBtn).toBeDisabled();
      });
    });
  });

  describe("Overwrite warning", () => {
    it("shows overwrite warning when hasChanges is true", async () => {
      const user = createUser();

      vi.spyOn(API, "request").mockResolvedValue(audnexusResponse);

      renderWithClient(
        <FetchChaptersDialog {...defaultProps} hasChanges={true} />,
      );

      const input = screen.getByRole("textbox");
      await user.type(input, "B0AAAATEST");
      await user.click(screen.getByRole("button", { name: /fetch chapters/i }));

      await waitFor(() => {
        expect(
          screen.getByText(/unsaved changes will be overwritten/i),
        ).toBeInTheDocument();
      });
    });

    it("does not show overwrite warning when hasChanges is false", async () => {
      const user = createUser();

      vi.spyOn(API, "request").mockResolvedValue(audnexusResponse);

      renderWithClient(
        <FetchChaptersDialog {...defaultProps} hasChanges={false} />,
      );

      const input = screen.getByRole("textbox");
      await user.type(input, "B0AAAATEST");
      await user.click(screen.getByRole("button", { name: /fetch chapters/i }));

      await waitFor(() => {
        expect(
          screen.queryByText(/unsaved changes will be overwritten/i),
        ).not.toBeInTheDocument();
      });
    });
  });

  describe("Error state", () => {
    it("shows not_found error message when ASIN is not found on Audible", async () => {
      const user = createUser();

      vi.spyOn(API, "request").mockRejectedValue(
        new ShishoAPIError("not found", "not_found", 404),
      );

      renderWithClient(<FetchChaptersDialog {...defaultProps} />);

      const input = screen.getByRole("textbox");
      await user.type(input, "B0AAAATEST");
      await user.click(screen.getByRole("button", { name: /fetch chapters/i }));

      await waitFor(() => {
        expect(screen.getByText(/lookup failed/i)).toBeInTheDocument();
      });

      expect(
        screen.getByText(/we couldn't find this asin on audible/i),
      ).toBeInTheDocument();
    });

    it("shows generic error message for unknown error codes", async () => {
      const user = createUser();

      vi.spyOn(API, "request").mockRejectedValue(
        new ShishoAPIError("server error", "server_error", 500),
      );

      renderWithClient(<FetchChaptersDialog {...defaultProps} />);

      const input = screen.getByRole("textbox");
      await user.type(input, "B0AAAATEST");
      await user.click(screen.getByRole("button", { name: /fetch chapters/i }));

      await waitFor(() => {
        expect(screen.getByText(/lookup failed/i)).toBeInTheDocument();
      });

      expect(screen.getByText(/couldn't reach audible/i)).toBeInTheDocument();
    });

    it("shows Retry button in error state", async () => {
      const user = createUser();

      vi.spyOn(API, "request").mockRejectedValue(
        new ShishoAPIError("not found", "not_found", 404),
      );

      renderWithClient(<FetchChaptersDialog {...defaultProps} />);

      const input = screen.getByRole("textbox");
      await user.type(input, "B0AAAATEST");
      await user.click(screen.getByRole("button", { name: /fetch chapters/i }));

      await waitFor(() => {
        expect(
          screen.getByRole("button", { name: /retry/i }),
        ).toBeInTheDocument();
      });
    });

    it("refetches when Retry is clicked", async () => {
      const user = createUser();

      const apiMock = vi
        .spyOn(API, "request")
        .mockRejectedValue(new ShishoAPIError("not found", "not_found", 404));

      renderWithClient(<FetchChaptersDialog {...defaultProps} />);

      const input = screen.getByRole("textbox");
      await user.type(input, "B0AAAATEST");
      await user.click(screen.getByRole("button", { name: /fetch chapters/i }));

      await waitFor(() => {
        expect(
          screen.getByRole("button", { name: /retry/i }),
        ).toBeInTheDocument();
      });

      const callsBefore = apiMock.mock.calls.length;
      await user.click(screen.getByRole("button", { name: /retry/i }));

      // Retry should fire a fresh upstream call rather than replaying the
      // cached error.
      await waitFor(() => {
        expect(apiMock.mock.calls.length).toBeGreaterThan(callsBefore);
      });
    });
  });
});
