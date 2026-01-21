import FileChaptersTab from "./FileChaptersTab";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useFileChapters } from "@/hooks/queries/chapters";
import { FileTypeM4B, type Chapter, type File } from "@/types";

// Mock the API hooks
vi.mock("@/hooks/queries/chapters", () => ({
  useFileChapters: vi.fn(),
  useUpdateFileChapters: vi.fn(() => ({
    mutate: vi.fn(),
    isPending: false,
  })),
}));

const mockUseFileChapters = vi.mocked(useFileChapters);

// Mock HTMLMediaElement.play and pause
beforeEach(() => {
  // Reset mocks
  vi.clearAllMocks();

  // Mock audio element methods - play returns a Promise
  window.HTMLMediaElement.prototype.play = vi.fn(() => Promise.resolve());
  window.HTMLMediaElement.prototype.pause = vi.fn();

  // Mock readyState to indicate audio is ready to play (HAVE_FUTURE_DATA = 3)
  Object.defineProperty(window.HTMLMediaElement.prototype, "readyState", {
    get: () => 4, // HAVE_ENOUGH_DATA
    configurable: true,
  });

  // Mock seeking to indicate not currently seeking
  Object.defineProperty(window.HTMLMediaElement.prototype, "seeking", {
    get: () => false,
    configurable: true,
  });
});

describe("FileChaptersTab - Play Promise handling", () => {
  const mockM4bFile: File = {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    book_id: 1,
    library_id: 1,
    file_type: FileTypeM4B,
    file_role: "main",
    filepath: "/test/book.m4b",
    filesize_bytes: 1000000,
    audiobook_duration_seconds: 3600,
  };

  const mockChapters: Chapter[] = [
    {
      id: 1,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      file_id: 1,
      title: "Chapter 1",
      sort_order: 0,
      start_timestamp_ms: 60000,
      children: [],
    },
  ];

  beforeEach(() => {
    mockUseFileChapters.mockReturnValue({
      data: mockChapters,
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useFileChapters>);
  });

  it("handles play() Promise rejection gracefully when interrupted by pause()", async () => {
    const user = userEvent.setup();

    // Track whether play was called so we only reject after it's called
    let playWasCalled = false;
    let rejectPlay: ((error: Error) => void) | null = null;

    // Create a new promise each time play is called
    const mockPlay = vi.fn(() => {
      playWasCalled = true;
      return new Promise<void>((_, reject) => {
        rejectPlay = reject;
      });
    });

    const mockPause = vi.fn(() => {
      // Only reject if play was called and we have a reject function
      if (playWasCalled && rejectPlay) {
        const error = new DOMException(
          "The play() request was interrupted by a call to pause().",
          "AbortError",
        );
        rejectPlay(error);
        rejectPlay = null; // Only reject once
      }
    });

    window.HTMLMediaElement.prototype.play = mockPlay;
    window.HTMLMediaElement.prototype.pause = mockPause;

    // Spy on console.error to ensure AbortError is not logged
    const consoleErrorSpy = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});

    renderWithProviders(
      <FileChaptersTab
        file={mockM4bFile}
        isEditing={true}
        onEditingChange={vi.fn()}
      />,
    );

    // Find the play button
    const allButtons = screen.getAllByRole("button");
    const playButton = allButtons.find((btn) =>
      btn.querySelector("svg")?.classList.contains("lucide-play"),
    );

    // Click play - this calls handleAudioPlay which:
    // 1. Calls handleAudioStop() (calls pause() - but no pending play yet)
    // 2. Calls play() - returns a new Promise
    await user.click(playButton!);

    // Now click play again on a different action that would trigger pause
    // This simulates rapidly clicking or the auto-stop
    mockPause();

    // Wait for Promise rejection to be processed
    await new Promise((resolve) => setTimeout(resolve, 10));

    // The AbortError should be handled gracefully and not logged as an error
    expect(consoleErrorSpy).not.toHaveBeenCalled();

    consoleErrorSpy.mockRestore();
  });
});

const createQueryClient = () =>
  new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

const renderWithProviders = (ui: React.ReactElement) => {
  const queryClient = createQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
};

describe("FileChaptersTab - M4B Playback", () => {
  const mockM4bFile: File = {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    book_id: 1,
    library_id: 1,
    file_type: FileTypeM4B,
    file_role: "main",
    filepath: "/test/book.m4b",
    filesize_bytes: 1000000,
    audiobook_duration_seconds: 3600, // 1 hour
  };

  const mockChapters: Chapter[] = [
    {
      id: 1,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      file_id: 1,
      title: "Chapter 1",
      sort_order: 0,
      start_timestamp_ms: 0,
      children: [],
    },
    {
      id: 2,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      file_id: 1,
      title: "Chapter 2",
      sort_order: 1,
      start_timestamp_ms: 60000,
      children: [],
    },
    {
      id: 3,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      file_id: 1,
      title: "Chapter 3",
      sort_order: 2,
      start_timestamp_ms: 120000,
      children: [],
    },
  ];

  beforeEach(() => {
    mockUseFileChapters.mockReturnValue({
      data: mockChapters,
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useFileChapters>);
  });

  describe("view mode", () => {
    it("renders all chapters with play buttons", () => {
      renderWithProviders(
        <FileChaptersTab
          file={mockM4bFile}
          isEditing={false}
          onEditingChange={vi.fn()}
        />,
      );

      expect(screen.getByText("Chapter 1")).toBeInTheDocument();
      expect(screen.getByText("Chapter 2")).toBeInTheDocument();
      expect(screen.getByText("Chapter 3")).toBeInTheDocument();

      // Each chapter should have a play button
      const buttons = screen.getAllByRole("button");
      expect(buttons.length).toBe(3);
    });

    it("plays audio when play button clicked", async () => {
      const user = userEvent.setup();

      renderWithProviders(
        <FileChaptersTab
          file={mockM4bFile}
          isEditing={false}
          onEditingChange={vi.fn()}
        />,
      );

      const buttons = screen.getAllByRole("button");
      await user.click(buttons[1]); // Click Chapter 2's play button

      expect(window.HTMLMediaElement.prototype.play).toHaveBeenCalled();
    });

    it("stops previous playback when clicking play on different chapter", async () => {
      const user = userEvent.setup();

      renderWithProviders(
        <FileChaptersTab
          file={mockM4bFile}
          isEditing={false}
          onEditingChange={vi.fn()}
        />,
      );

      const buttons = screen.getAllByRole("button");

      // Play Chapter 1
      await user.click(buttons[0]);
      expect(window.HTMLMediaElement.prototype.play).toHaveBeenCalledTimes(1);

      // Play Chapter 2 - should stop Chapter 1 first
      await user.click(buttons[1]);
      expect(window.HTMLMediaElement.prototype.pause).toHaveBeenCalled();
      expect(window.HTMLMediaElement.prototype.play).toHaveBeenCalledTimes(2);
    });

    it("stops playback when stop button clicked", async () => {
      const user = userEvent.setup();

      renderWithProviders(
        <FileChaptersTab
          file={mockM4bFile}
          isEditing={false}
          onEditingChange={vi.fn()}
        />,
      );

      const buttons = screen.getAllByRole("button");

      // Play Chapter 1
      await user.click(buttons[0]);

      // Click again to stop (now it's a stop button)
      await user.click(buttons[0]);

      expect(window.HTMLMediaElement.prototype.pause).toHaveBeenCalled();
    });
  });

  describe("edit mode - playback with reordering", () => {
    it("stops playback when entering edit mode", async () => {
      const user = userEvent.setup();
      const onEditingChange = vi.fn();

      const { rerender } = renderWithProviders(
        <FileChaptersTab
          file={mockM4bFile}
          isEditing={false}
          onEditingChange={onEditingChange}
        />,
      );

      // Play a chapter
      const buttons = screen.getAllByRole("button");
      await user.click(buttons[0]);

      expect(window.HTMLMediaElement.prototype.play).toHaveBeenCalled();

      // Enter edit mode
      rerender(
        <QueryClientProvider client={createQueryClient()}>
          <MemoryRouter>
            <FileChaptersTab
              file={mockM4bFile}
              isEditing={true}
              onEditingChange={onEditingChange}
            />
          </MemoryRouter>
        </QueryClientProvider>,
      );

      // Playback should have been stopped
      await waitFor(() => {
        expect(window.HTMLMediaElement.prototype.pause).toHaveBeenCalled();
      });
    });

    it("renders chapters with play buttons in edit mode", async () => {
      renderWithProviders(
        <FileChaptersTab
          file={mockM4bFile}
          isEditing={true}
          onEditingChange={vi.fn()}
        />,
      );

      // Should still have chapter rows
      expect(screen.getByDisplayValue("Chapter 1")).toBeInTheDocument();
      expect(screen.getByDisplayValue("Chapter 2")).toBeInTheDocument();
      expect(screen.getByDisplayValue("Chapter 3")).toBeInTheDocument();
    });
  });

  describe("index-based playback tracking", () => {
    it("only one chapter shows as playing at a time", async () => {
      const user = userEvent.setup();

      renderWithProviders(
        <FileChaptersTab
          file={mockM4bFile}
          isEditing={false}
          onEditingChange={vi.fn()}
        />,
      );

      const buttons = screen.getAllByRole("button");

      // Play first chapter
      await user.click(buttons[0]);
      expect(window.HTMLMediaElement.prototype.play).toHaveBeenCalledTimes(1);

      // Play second chapter - should stop first and play second
      await user.click(buttons[1]);

      // pause should be called (to stop first chapter)
      expect(window.HTMLMediaElement.prototype.pause).toHaveBeenCalled();
      // play should be called twice (once for each chapter)
      expect(window.HTMLMediaElement.prototype.play).toHaveBeenCalledTimes(2);
    });
  });

  describe("empty state", () => {
    it("shows empty state when no chapters", () => {
      mockUseFileChapters.mockReturnValue({
        data: [],
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as ReturnType<typeof useFileChapters>);

      renderWithProviders(
        <FileChaptersTab
          file={mockM4bFile}
          isEditing={false}
          onEditingChange={vi.fn()}
        />,
      );

      expect(screen.getByText("No chapters")).toBeInTheDocument();
      expect(screen.getByText("Add Chapter")).toBeInTheDocument();
    });
  });

  describe("loading state", () => {
    it("shows loading spinner when loading", () => {
      mockUseFileChapters.mockReturnValue({
        data: undefined,
        isLoading: true,
        isError: false,
        error: null,
      } as ReturnType<typeof useFileChapters>);

      renderWithProviders(
        <FileChaptersTab
          file={mockM4bFile}
          isEditing={false}
          onEditingChange={vi.fn()}
        />,
      );

      // Should show some loading indicator
      expect(screen.queryByText("Chapter 1")).not.toBeInTheDocument();
    });
  });

  describe("error state", () => {
    it("shows error message when fetch fails", () => {
      mockUseFileChapters.mockReturnValue({
        data: undefined,
        isLoading: false,
        isError: true,
        error: new Error("Failed to fetch"),
      } as ReturnType<typeof useFileChapters>);

      renderWithProviders(
        <FileChaptersTab
          file={mockM4bFile}
          isEditing={false}
          onEditingChange={vi.fn()}
        />,
      );

      expect(screen.getByText("Failed to load chapters")).toBeInTheDocument();
    });
  });
});

describe("FileChaptersTab - Edit mode play after timestamp change", () => {
  const mockM4bFile: File = {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    book_id: 1,
    library_id: 1,
    file_type: FileTypeM4B,
    file_role: "main",
    filepath: "/test/book.m4b",
    filesize_bytes: 1000000,
    audiobook_duration_seconds: 3600,
  };

  const mockChapters: Chapter[] = [
    {
      id: 1,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      file_id: 1,
      title: "Chapter 1",
      sort_order: 0,
      start_timestamp_ms: 60000, // 1 minute
      children: [],
    },
  ];

  beforeEach(() => {
    mockUseFileChapters.mockReturnValue({
      data: mockChapters,
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useFileChapters>);
  });

  it("plays audio after clicking minus button to decrement timestamp", async () => {
    const user = userEvent.setup();

    renderWithProviders(
      <FileChaptersTab
        file={mockM4bFile}
        isEditing={true}
        onEditingChange={vi.fn()}
      />,
    );

    // Find the minus button (decrements timestamp by 1 second)
    const minusButton = screen.getByTitle("Subtract 1 second");
    await user.click(minusButton);

    // Find the play button
    const allButtons = screen.getAllByRole("button");
    const playButton = allButtons.find((btn) =>
      btn.querySelector("svg")?.classList.contains("lucide-play"),
    );

    expect(playButton).toBeDefined();
    await user.click(playButton!);

    // Play should have been called
    expect(window.HTMLMediaElement.prototype.play).toHaveBeenCalled();
  });

  it("plays audio after clicking plus button to increment timestamp", async () => {
    const user = userEvent.setup();

    renderWithProviders(
      <FileChaptersTab
        file={mockM4bFile}
        isEditing={true}
        onEditingChange={vi.fn()}
      />,
    );

    // Find the plus button (increments timestamp by 1 second)
    const plusButton = screen.getByTitle("Add 1 second");
    await user.click(plusButton);

    // Find the play button
    const allButtons = screen.getAllByRole("button");
    const playButton = allButtons.find((btn) =>
      btn.querySelector("svg")?.classList.contains("lucide-play"),
    );

    expect(playButton).toBeDefined();
    await user.click(playButton!);

    // Play should have been called
    expect(window.HTMLMediaElement.prototype.play).toHaveBeenCalled();
  });

  it("plays audio with updated timestamp after decrementing (state updates correctly)", async () => {
    const user = userEvent.setup();

    // Mock currentTime setter to verify correct timestamp is used
    let capturedCurrentTime: number | undefined;
    Object.defineProperty(window.HTMLMediaElement.prototype, "currentTime", {
      set: (value: number) => {
        capturedCurrentTime = value;
      },
      get: () => capturedCurrentTime ?? 0,
      configurable: true,
    });

    renderWithProviders(
      <FileChaptersTab
        file={mockM4bFile}
        isEditing={true}
        onEditingChange={vi.fn()}
      />,
    );

    // Chapter starts at 60000ms (1 minute = 60 seconds)
    // Click minus to decrement by 1 second (should be 59 seconds)
    const minusButton = screen.getByTitle("Subtract 1 second");
    await user.click(minusButton);

    // Find and click the play button
    const allButtons = screen.getAllByRole("button");
    const playButton = allButtons.find((btn) =>
      btn.querySelector("svg")?.classList.contains("lucide-play"),
    );

    await user.click(playButton!);

    // The currentTime should be set to 59 seconds (59000ms / 1000)
    // This verifies the state was updated correctly and the new timestamp is used
    expect(capturedCurrentTime).toBe(59);
  });

  it("can play after clicking minus while already playing", async () => {
    const user = userEvent.setup();

    renderWithProviders(
      <FileChaptersTab
        file={mockM4bFile}
        isEditing={true}
        onEditingChange={vi.fn()}
      />,
    );

    // Find and click the play button to start playing
    const allButtons = screen.getAllByRole("button");
    const playButton = allButtons.find((btn) =>
      btn.querySelector("svg")?.classList.contains("lucide-play"),
    );

    await user.click(playButton!);
    expect(window.HTMLMediaElement.prototype.play).toHaveBeenCalledTimes(1);

    // Now click minus while playing
    const minusButton = screen.getByTitle("Subtract 1 second");
    await user.click(minusButton);

    // Find the play/stop button again (it might be stop now since we were playing)
    const buttonsAfterMinus = screen.getAllByRole("button");
    const playOrStopButton = buttonsAfterMinus.find(
      (btn) =>
        btn.querySelector("svg")?.classList.contains("lucide-play") ||
        btn.querySelector("svg")?.classList.contains("lucide-square"),
    );

    // Click play/stop - should be able to play again without errors
    await user.click(playOrStopButton!);

    // Should have either stopped (if it was stop button) or played again
    // Either way, no errors should occur
    expect(window.HTMLMediaElement.prototype.pause).toHaveBeenCalled();
  });
});

describe("FileChaptersTab - Timestamp reorder stops playback", () => {
  const mockM4bFile: File = {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    book_id: 1,
    library_id: 1,
    file_type: FileTypeM4B,
    file_role: "main",
    filepath: "/test/book.m4b",
    filesize_bytes: 1000000,
    audiobook_duration_seconds: 3600,
  };

  const mockChapters: Chapter[] = [
    {
      id: 1,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      file_id: 1,
      title: "Chapter 1",
      sort_order: 0,
      start_timestamp_ms: 0,
      children: [],
    },
    {
      id: 2,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      file_id: 1,
      title: "Chapter 2",
      sort_order: 1,
      start_timestamp_ms: 60000,
      children: [],
    },
  ];

  beforeEach(() => {
    mockUseFileChapters.mockReturnValue({
      data: mockChapters,
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useFileChapters>);
  });

  it("stops playback when timestamp input loses focus (triggers reorder)", async () => {
    const user = userEvent.setup();

    renderWithProviders(
      <FileChaptersTab
        file={mockM4bFile}
        isEditing={true}
        onEditingChange={vi.fn()}
      />,
    );

    // Find play buttons in edit mode - they're part of the timestamp controls
    // In edit mode, there are multiple buttons per row (play, -, +, delete)
    const allButtons = screen.getAllByRole("button");

    // Find a play button by looking for one that doesn't have specific text
    // Play buttons are the ones without visible text content
    const playButtons = allButtons.filter((btn) => {
      const svg = btn.querySelector("svg");
      return svg?.classList.contains("lucide-play");
    });

    if (playButtons.length > 0) {
      // Play the first chapter
      await user.click(playButtons[0]);
      expect(window.HTMLMediaElement.prototype.play).toHaveBeenCalled();

      // Find timestamp inputs
      const timestampInputs = screen.getAllByRole("textbox");
      const chapterTimestampInputs = timestampInputs.filter((input) =>
        (input as HTMLInputElement).value.includes(":"),
      );

      if (chapterTimestampInputs.length > 0) {
        // Change timestamp and blur to trigger reorder
        await user.clear(chapterTimestampInputs[0]);
        await user.type(chapterTimestampInputs[0], "00:02:00.000");
        await user.tab(); // Blur the input

        // Playback should be stopped due to reorder
        await waitFor(() => {
          expect(window.HTMLMediaElement.prototype.pause).toHaveBeenCalled();
        });
      }
    }
  });
});
