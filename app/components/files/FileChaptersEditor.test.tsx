import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef, useState } from "react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  useFileChapters,
  useUpdateFileChapters,
} from "@/hooks/queries/chapters";
import { ShishoAPIError } from "@/libraries/api";
import { ALL_PERMISSIONS, setAuth } from "@/testing/auth";
import {
  FileTypeAZW3,
  FileTypeEPUB,
  FileTypeM4B,
  FileTypeMOBI,
  FileTypePDF,
  type Chapter,
  type File,
  type FileType,
} from "@/types";

import FileChaptersEditor, {
  type FileChaptersEditorHandle,
} from "./FileChaptersEditor";

// Query hooks check the role's permissions; this test grants them all.
vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

setAuth({ permissions: ALL_PERMISSIONS });

const createUser = () =>
  userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

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

describe("FileChaptersEditor - Play Promise handling", () => {
  const mockM4bFile: File = {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    book_id: 1,
    library_id: 1,
    file_type: FileTypeM4B,
    file_role: "main",
    filepath: "/test/book.m4b",
    display_name: "book.m4b",
    filesize_bytes: 1000000,
    audiobook_duration_seconds: 3600,
    is_preferred_cover: false,
    reachable_file_types: [],
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
    const user = createUser();

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
      <FileChaptersEditor
        canEdit
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

describe("FileChaptersEditor - M4B Playback", () => {
  const mockM4bFile: File = {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    book_id: 1,
    library_id: 1,
    file_type: FileTypeM4B,
    file_role: "main",
    filepath: "/test/book.m4b",
    display_name: "book.m4b",
    filesize_bytes: 1000000,
    audiobook_duration_seconds: 3600, // 1 hour
    is_preferred_cover: false,
    reachable_file_types: [],
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
        <FileChaptersEditor
          canEdit
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
      const user = createUser();

      renderWithProviders(
        <FileChaptersEditor
          canEdit
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
      const user = createUser();

      renderWithProviders(
        <FileChaptersEditor
          canEdit
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
      const user = createUser();

      renderWithProviders(
        <FileChaptersEditor
          canEdit
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
      const user = createUser();
      const onEditingChange = vi.fn();

      const { rerender } = renderWithProviders(
        <FileChaptersEditor
          canEdit
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
            <FileChaptersEditor
              canEdit
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
        <FileChaptersEditor
          canEdit
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
      const user = createUser();

      renderWithProviders(
        <FileChaptersEditor
          canEdit
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
    it("shows the empty state and leaves its actions to the page's bar", () => {
      mockUseFileChapters.mockReturnValue({
        data: [],
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as ReturnType<typeof useFileChapters>);

      renderWithProviders(
        <FileChaptersEditor
          canEdit
          file={mockM4bFile}
          isEditing={false}
          onEditingChange={vi.fn()}
        />,
      );

      expect(screen.getByText("No chapters")).toBeInTheDocument();
      expect(screen.queryByRole("button")).not.toBeInTheDocument();
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
        <FileChaptersEditor
          canEdit
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
        <FileChaptersEditor
          canEdit
          file={mockM4bFile}
          isEditing={false}
          onEditingChange={vi.fn()}
        />,
      );

      expect(screen.getByRole("alert")).toHaveTextContent(
        /^Failed to load chapters/,
      );
    });

    it("shows the server's message and retries on request", async () => {
      const refetch = vi.fn();
      mockUseFileChapters.mockReturnValue({
        data: undefined,
        isLoading: false,
        isError: true,
        isFetching: false,
        isEnabled: true,
        refetch,
        error: new ShishoAPIError(
          "Chapters are unreadable for this file",
          "unprocessable",
          422,
        ),
      } as unknown as ReturnType<typeof useFileChapters>);

      renderWithProviders(
        <FileChaptersEditor
          canEdit
          file={mockM4bFile}
          isEditing={false}
          onEditingChange={vi.fn()}
        />,
      );

      expect(screen.getByRole("alert")).toHaveTextContent(
        /^Chapters are unreadable for this file/,
      );
      expect(
        screen.queryByText("Failed to load chapters"),
      ).not.toBeInTheDocument();

      await userEvent.click(screen.getByRole("button", { name: "Retry" }));
      expect(refetch).toHaveBeenCalledTimes(1);
    });
  });
});

describe("FileChaptersEditor - Edit mode play after timestamp change", () => {
  const mockM4bFile: File = {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    book_id: 1,
    library_id: 1,
    file_type: FileTypeM4B,
    file_role: "main",
    filepath: "/test/book.m4b",
    display_name: "book.m4b",
    filesize_bytes: 1000000,
    audiobook_duration_seconds: 3600,
    is_preferred_cover: false,
    reachable_file_types: [],
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
    const user = createUser();

    renderWithProviders(
      <FileChaptersEditor
        canEdit
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
    const user = createUser();

    renderWithProviders(
      <FileChaptersEditor
        canEdit
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
    const user = createUser();

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
      <FileChaptersEditor
        canEdit
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
    const user = createUser();

    renderWithProviders(
      <FileChaptersEditor
        canEdit
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

describe("FileChaptersEditor - PDF chapter page increment regression", () => {
  const mockPdfFile: File = {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    book_id: 1,
    library_id: 1,
    file_type: FileTypePDF,
    file_role: "main",
    filepath: "/test/book.pdf",
    display_name: "book.pdf",
    filesize_bytes: 1000000,
    page_count: 100,
    is_preferred_cover: false,
    reachable_file_types: [],
  };

  const mockChapters: Chapter[] = [
    {
      id: 1,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      file_id: 1,
      title: "Alpha",
      sort_order: 0,
      start_page: 0,
      children: [],
    },
    {
      id: 2,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      file_id: 1,
      title: "Bravo",
      sort_order: 1,
      start_page: 5,
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

  it("clicking next-page on the first chapter past the second's page does not mutate the second chapter", async () => {
    // Regression: live sort-on-blur combined with index-based React keys
    // meant that once the first chapter's page crossed the second's, the DOM
    // element at screen position 0 got reused for the (now-first) second
    // chapter, and the user's next click on that position silently
    // incremented the wrong chapter.
    const user = createUser();

    renderWithProviders(
      <FileChaptersEditor
        canEdit
        file={mockPdfFile}
        isEditing={true}
        onEditingChange={vi.fn()}
      />,
    );

    // Wait for edit mode to initialize from the mocked chapters.
    await screen.findByDisplayValue("Alpha");

    // Find the "Next page" buttons — one per chapter row.
    const nextButtons = screen.getAllByRole("button", { name: /next page/i });
    expect(nextButtons.length).toBe(2);

    // Click Alpha's + button 7 times. This would push Alpha from page 0 to
    // page 7, crossing Bravo's page 5.
    for (let i = 0; i < 7; i++) {
      await user.click(nextButtons[0]);
    }

    // The first row should still be Alpha with its page advanced to 8
    // (0-indexed 7 + 1 for display). Bravo should be untouched at display 6.
    const titleInputs = screen.getAllByPlaceholderText(
      "Chapter title",
    ) as HTMLInputElement[];
    expect(titleInputs[0].value).toBe("Alpha");
    expect(titleInputs[1].value).toBe("Bravo");

    const pageInputs = screen.getAllByRole("spinbutton") as HTMLInputElement[];
    expect(pageInputs[0].value).toBe("8");
    expect(pageInputs[1].value).toBe("6");
  });

  it("typing a new page number and blurring reorders the chapters", async () => {
    // The companion behavior: typing a page number into an input and tabbing
    // out is an explicit "commit" gesture, so the parent does reorder on
    // blur. Only button clicks intentionally skip the reorder.
    const user = createUser();

    renderWithProviders(
      <FileChaptersEditor
        canEdit
        file={mockPdfFile}
        isEditing={true}
        onEditingChange={vi.fn()}
      />,
    );

    await screen.findByDisplayValue("Alpha");

    const pageInputs = screen.getAllByRole("spinbutton") as HTMLInputElement[];
    expect(pageInputs[0].value).toBe("1"); // Alpha at display 1 (0-indexed 0)
    expect(pageInputs[1].value).toBe("6"); // Bravo at display 6 (0-indexed 5)

    // Retarget Alpha to page 10 (display 10 → 0-indexed 9).
    await user.clear(pageInputs[0]);
    await user.type(pageInputs[0], "10");
    await user.tab(); // Blur the input → parent sorts by start_page.

    // After reorder, Bravo (page 5) should be first and Alpha (page 9)
    // should be second. The title inputs and the page inputs both move.
    const titleInputsAfter = screen.getAllByPlaceholderText(
      "Chapter title",
    ) as HTMLInputElement[];
    expect(titleInputsAfter[0].value).toBe("Bravo");
    expect(titleInputsAfter[1].value).toBe("Alpha");

    const pageInputsAfter = screen.getAllByRole(
      "spinbutton",
    ) as HTMLInputElement[];
    expect(pageInputsAfter[0].value).toBe("6");
    expect(pageInputsAfter[1].value).toBe("10");
  });
});

describe("FileChaptersEditor - adding a PDF chapter", () => {
  const shortPdf: File = {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    book_id: 1,
    library_id: 1,
    file_type: FileTypePDF,
    file_role: "main",
    filepath: "/test/short.pdf",
    display_name: "short.pdf",
    filesize_bytes: 1000,
    page_count: 6,
    is_preferred_cover: false,
    reachable_file_types: [],
  };

  const chapterAt = (id: number, title: string, page: number): Chapter => ({
    id,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    file_id: 1,
    title,
    sort_order: id - 1,
    start_page: page,
    children: [],
  });

  const addChapter = async (chapters: Chapter[]) => {
    mockUseFileChapters.mockReturnValue({
      data: chapters,
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useFileChapters>);
    const ref = createRef<FileChaptersEditorHandle>();
    renderWithProviders(
      <FileChaptersEditor
        canEdit
        file={shortPdf}
        isEditing={true}
        onEditingChange={vi.fn()}
        ref={ref}
      />,
    );
    await screen.findByDisplayValue(chapters[0].title);
    // The file page's chapter bar adds through the handle.
    act(() => ref.current!.addChapter());
    return screen.getAllByRole("spinbutton") as HTMLInputElement[];
  };

  it("starts the new chapter on the page after the last one", async () => {
    const pages = await addChapter([chapterAt(1, "Chapter 1", 2)]);
    expect(pages.map((p) => p.value)).toEqual(["3", "4"]);
  });

  it("adds and saves a first chapter on a PDF that has none", async () => {
    const mutate = vi.fn();
    vi.mocked(useUpdateFileChapters).mockReturnValue({
      mutate,
      isPending: false,
    } as never);
    mockUseFileChapters.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      error: null,
    } as unknown as ReturnType<typeof useFileChapters>);
    const ref = createRef<FileChaptersEditorHandle>();
    const states: { canSave: boolean; hasChanges: boolean }[] = [];
    renderWithProviders(
      <FileChaptersEditor
        canEdit
        file={shortPdf}
        isEditing={true}
        onActionStateChange={(state) => states.push(state)}
        onEditingChange={vi.fn()}
        ref={ref}
      />,
    );
    expect(screen.getByText("No chapters")).toBeInTheDocument();

    act(() => ref.current!.addChapter());
    const [page] = screen.getAllByRole("spinbutton") as HTMLInputElement[];
    expect(page.value).toBe("1");
    expect(states[states.length - 1]).toMatchObject({
      canSave: true,
      hasChanges: true,
    });

    act(() => ref.current!.save());
    expect(mutate).toHaveBeenCalledWith(
      { chapters: [expect.objectContaining({ start_page: 0 })] },
      expect.anything(),
    );
  });

  it("never starts the new chapter past the last page", async () => {
    // The last chapter is on page 6 of 6, so page 7 does not exist.
    const pages = await addChapter([
      chapterAt(1, "Chapter 1", 0),
      chapterAt(2, "Chapter 2", 5),
    ]);
    expect(pages.map((p) => p.value)).toEqual(["1", "6", "6"]);
  });
});

describe("FileChaptersEditor - uncovered pages banner", () => {
  const mockPdfFile: File = {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    book_id: 1,
    library_id: 1,
    file_type: FileTypePDF,
    file_role: "main",
    filepath: "/test/book.pdf",
    display_name: "book.pdf",
    filesize_bytes: 1000000,
    page_count: 100,
    is_preferred_cover: false,
    reachable_file_types: [],
  };

  beforeEach(() => {
    mockUseFileChapters.mockReturnValue({
      data: [
        {
          id: 1,
          created_at: "2024-01-01T00:00:00Z",
          updated_at: "2024-01-01T00:00:00Z",
          file_id: 1,
          title: "Alpha",
          sort_order: 0,
          start_page: 3,
          children: [],
        },
      ],
      isLoading: false,
      isError: false,
      error: null,
    } as unknown as ReturnType<typeof useFileChapters>);
  });

  it("offers to add a chapter when the user can edit", async () => {
    const user = createUser();
    const onEditingChange = vi.fn();
    renderWithProviders(
      <FileChaptersEditor
        canEdit
        file={mockPdfFile}
        isEditing={false}
        onEditingChange={onEditingChange}
      />,
    );
    await user.click(
      screen.getByRole("button", { name: /not in any chapter/ }),
    );
    expect(onEditingChange).toHaveBeenCalledWith(true);
  });

  it("shows the warning without an action when the user cannot edit", () => {
    renderWithProviders(
      <FileChaptersEditor
        canEdit={false}
        file={mockPdfFile}
        isEditing={false}
        onEditingChange={vi.fn()}
      />,
    );
    expect(
      screen.getByText(/Pages 1-3 not in any chapter/),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /not in any chapter/ }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("Click to add chapter")).not.toBeInTheDocument();
  });
});

describe("FileChaptersEditor - EPUB heading left without chapters", () => {
  const mockEpubFile: File = {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    book_id: 1,
    library_id: 1,
    file_type: FileTypeEPUB,
    file_role: "main",
    filepath: "/test/book.epub",
    display_name: "book.epub",
    filesize_bytes: 1000000,
    is_preferred_cover: false,
    reachable_file_types: [],
  };

  const chapter = (
    id: number,
    title: string,
    href: string | undefined,
    children: Chapter[] = [],
  ): Chapter => ({
    id,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    file_id: 1,
    title,
    sort_order: id,
    href,
    children,
  });

  // Rows in order: Part One (heading), One A, Part Two (linked), Two A,
  // Already Empty (a heading the book itself leaves empty).
  const mockChapters: Chapter[] = [
    chapter(1, "Part One", undefined, [chapter(2, "One A", "a.xhtml")]),
    chapter(3, "Part Two", "p2.xhtml", [chapter(4, "Two A", "b.xhtml")]),
    chapter(5, "Already Empty", undefined),
  ];

  beforeEach(() => {
    mockUseFileChapters.mockReturnValue({
      data: mockChapters,
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useFileChapters>);
  });

  const renderEditing = () => {
    const onActionStateChange = vi.fn();
    renderWithProviders(
      <FileChaptersEditor
        canEdit
        file={mockEpubFile}
        isEditing={true}
        onActionStateChange={onActionStateChange}
        onEditingChange={vi.fn()}
      />,
    );
    const canSave = () =>
      onActionStateChange.mock.lastCall?.[0]?.canSave as boolean | undefined;
    return { canSave };
  };

  const deleteButtons = () =>
    screen.getAllByRole("button", { name: "Delete chapter" });

  it("blocks saving until a heading emptied in this edit is deleted", async () => {
    const user = createUser();
    const { canSave } = renderEditing();
    await screen.findByDisplayValue("Part One");
    expect(
      screen.queryByText(/no chapters are left under this heading/i),
    ).not.toBeInTheDocument();
    expect(canSave()).toBe(true);

    await user.click(deleteButtons()[1]); // One A

    const message = await screen.findByText(
      /no chapters are left under this heading/i,
    );
    expect(screen.getByDisplayValue("Part One")).toHaveAccessibleDescription(
      message.textContent ?? "",
    );
    await waitFor(() => expect(canSave()).toBe(false));

    await user.click(deleteButtons()[0]); // Part One, now without children

    expect(
      screen.queryByText(/no chapters are left under this heading/i),
    ).not.toBeInTheDocument();
    await waitFor(() => expect(canSave()).toBe(true));
  });

  it("does not flag a linked entry left without chapters", async () => {
    const user = createUser();
    const { canSave } = renderEditing();
    await screen.findByDisplayValue("Part Two");

    await user.click(deleteButtons()[3]); // Two A

    expect(
      screen.queryByText(/no chapters are left under this heading/i),
    ).not.toBeInTheDocument();
    await waitFor(() => expect(canSave()).toBe(true));
  });
});

describe("FileChaptersEditor - EPUB chapters nested three deep", () => {
  const mockEpubFile: File = {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    book_id: 1,
    library_id: 1,
    file_type: FileTypeEPUB,
    file_role: "main",
    filepath: "/test/book.epub",
    display_name: "book.epub",
    filesize_bytes: 1000000,
    is_preferred_cover: false,
    reachable_file_types: [],
  };

  const chapter = (
    id: number,
    title: string,
    children: Chapter[] = [],
  ): Chapter => ({
    id,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    file_id: 1,
    title,
    sort_order: id,
    href: `${id}.xhtml`,
    children,
  });

  beforeEach(() => {
    mockUseFileChapters.mockReturnValue({
      data: [
        chapter(1, "Part", [
          chapter(2, "Chapter", [
            chapter(3, "Section A"),
            chapter(4, "Section B"),
          ]),
        ]),
      ],
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useFileChapters>);
  });

  const renderEditing = () =>
    renderWithProviders(
      <FileChaptersEditor
        canEdit
        file={mockEpubFile}
        isEditing={true}
        onEditingChange={vi.fn()}
      />,
    );

  it("renames a chapter at the third level", async () => {
    const user = createUser();
    renderEditing();
    const input = await screen.findByDisplayValue("Section A");

    await user.clear(input);
    await user.type(input, "Opening");

    expect(screen.getByDisplayValue("Opening")).toBeInTheDocument();
  });

  it("deletes a chapter at the third level", async () => {
    const user = createUser();
    renderEditing();
    await screen.findByDisplayValue("Section A");

    // Rows in order: Part, Chapter, Section A, Section B.
    await user.click(
      screen.getAllByRole("button", { name: "Delete chapter" })[3],
    );

    expect(screen.queryByDisplayValue("Section B")).not.toBeInTheDocument();
    expect(screen.getByDisplayValue("Section A")).toBeInTheDocument();
  });
});

describe("FileChaptersEditor - EPUB edit state", () => {
  const mockEpubFile: File = {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    book_id: 1,
    library_id: 1,
    file_type: FileTypeEPUB,
    file_role: "main",
    filepath: "/test/book.epub",
    display_name: "book.epub",
    filesize_bytes: 1000000,
    is_preferred_cover: false,
    reachable_file_types: [],
  };

  const chapter = (
    id: number,
    title: string,
    href: string | undefined,
    children: Chapter[] = [],
  ): Chapter => ({
    id,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    file_id: 1,
    title,
    sort_order: id,
    href,
    children,
  });

  const useChapters = (chapters: Chapter[]) =>
    mockUseFileChapters.mockReturnValue({
      data: chapters,
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useFileChapters>);

  // Lets a test leave and re-enter edit mode the way the file page does.
  const Harness = ({
    tabRef,
  }: {
    tabRef?: React.Ref<FileChaptersEditorHandle>;
  }) => {
    const [editing, setEditing] = useState(true);
    return (
      <>
        <button onClick={() => setEditing((e) => !e)} type="button">
          Toggle editing
        </button>
        <FileChaptersEditor
          canEdit
          file={mockEpubFile}
          isEditing={editing}
          onEditingChange={setEditing}
          ref={tabRef}
        />
      </>
    );
  };

  const deleteButtons = () =>
    screen.getAllByRole("button", { name: "Delete chapter" });

  it("keeps each row's expanded state when a sibling above it is deleted", async () => {
    useChapters([
      chapter(1, "Part", "p.xhtml", [
        chapter(2, "Ch1", "a.xhtml", [chapter(3, "S1", "a.xhtml#1")]),
        chapter(4, "Ch2", "b.xhtml", [chapter(5, "S2", "b.xhtml#1")]),
      ]),
    ]);
    const user = createUser();
    renderWithProviders(<Harness />);
    await screen.findByDisplayValue("Ch1");

    // Collapse Ch1, then delete it (it has a subchapter, so confirm).
    await user.click(
      screen.getAllByRole("button", { name: "Hide subchapters" })[1],
    );
    await user.click(deleteButtons()[1]);
    await user.click(screen.getByRole("button", { name: "Delete" }));

    expect(screen.queryByDisplayValue("Ch1")).not.toBeInTheDocument();
    // Ch2 was expanded and stays expanded.
    expect(screen.getByDisplayValue("S2")).toBeInTheDocument();
  });

  it("does not save while a heading is left without chapters", async () => {
    const mutate = vi.fn();
    vi.mocked(useUpdateFileChapters).mockReturnValue({
      mutate,
      isPending: false,
    } as never);
    useChapters([
      chapter(1, "Part", undefined, [chapter(2, "Only", "a.xhtml")]),
      chapter(3, "After", "b.xhtml"),
    ]);
    const user = createUser();
    const tabRef = createRef<FileChaptersEditorHandle>();
    renderWithProviders(<Harness tabRef={tabRef} />);
    await screen.findByDisplayValue("Only");

    await user.click(deleteButtons()[1]); // Only
    await screen.findByText(/no chapters are left under this heading/i);
    act(() => tabRef.current?.save());
    expect(mutate).not.toHaveBeenCalled();

    await user.click(deleteButtons()[0]); // Part
    act(() => tabRef.current?.save());
    expect(mutate).toHaveBeenCalledTimes(1);
  });

  it("flags an emptied heading after leaving and re-entering edit mode", async () => {
    useChapters([
      chapter(1, "Part", undefined, [chapter(2, "Only", "a.xhtml")]),
    ]);
    const user = createUser();
    renderWithProviders(<Harness />);
    await screen.findByDisplayValue("Only");

    const toggle = screen.getByRole("button", { name: "Toggle editing" });
    await user.click(toggle);
    await user.click(toggle);
    await screen.findByDisplayValue("Only");

    await user.click(deleteButtons()[1]);
    expect(
      await screen.findByText(/no chapters are left under this heading/i),
    ).toBeInTheDocument();
  });

  it("flags a nested heading left without chapters", async () => {
    useChapters([
      chapter(1, "Part", "p.xhtml", [
        chapter(2, "Section", undefined, [chapter(3, "Only", "a.xhtml")]),
      ]),
    ]);
    const user = createUser();
    renderWithProviders(<Harness />);
    await screen.findByDisplayValue("Only");

    await user.click(deleteButtons()[2]); // Only

    const message = await screen.findByText(
      /no chapters are left under this heading/i,
    );
    expect(screen.getByDisplayValue("Section")).toHaveAccessibleDescription(
      message.textContent ?? "",
    );
  });
});

describe.each<FileType>([FileTypeMOBI, FileTypeAZW3])(
  "FileChaptersEditor - %s chapters",
  (fileType) => {
    const file: File = {
      id: 1,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      book_id: 1,
      library_id: 1,
      file_type: fileType,
      file_role: "main",
      filepath: `/test/book.${fileType}`,
      display_name: `book.${fileType}`,
      filesize_bytes: 1000000,
      is_preferred_cover: false,
      reachable_file_types: [],
    };

    const chapter = (
      id: number,
      title: string,
      children: Chapter[] = [],
    ): Chapter => ({
      id,
      created_at: "2024-01-01T00:00:00Z",
      updated_at: "2024-01-01T00:00:00Z",
      file_id: 1,
      title,
      sort_order: id,
      href: `kindle:pos:fid:000${id}:off:0000000000`,
      children,
    });

    beforeEach(() => {
      mockUseFileChapters.mockReturnValue({
        data: [
          chapter(1, "Part One", [
            chapter(2, "Chapter 1"),
            chapter(3, "Chapter 2", [chapter(4, "Scene")]),
          ]),
          chapter(5, "Epilogue"),
        ],
        isLoading: false,
        isError: false,
        error: null,
      } as ReturnType<typeof useFileChapters>);
    });

    it("shows nested chapters with expand toggles when viewing", async () => {
      renderWithProviders(
        <FileChaptersEditor
          canEdit
          file={file}
          isEditing={false}
          onEditingChange={vi.fn()}
        />,
      );
      expect(await screen.findByText("Scene")).toBeInTheDocument();
      expect(
        screen.getAllByRole("button", { name: "Hide subchapters" }),
      ).toHaveLength(2);
    });

    it("renames and deletes nested chapters with the edit notice", async () => {
      const mutate = vi.fn();
      vi.mocked(useUpdateFileChapters).mockReturnValue({
        mutate,
        isPending: false,
      } as never);
      const user = createUser();
      const tabRef = createRef<FileChaptersEditorHandle>();
      renderWithProviders(
        <FileChaptersEditor
          canEdit
          file={file}
          isEditing
          onEditingChange={vi.fn()}
          ref={tabRef}
        />,
      );

      const title = await screen.findByDisplayValue("Chapter 1");
      expect(
        screen.getByText(/only renaming and deleting existing chapters/i),
      ).toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /add chapter/i }),
      ).not.toBeInTheDocument();

      await user.clear(title);
      await user.type(title, "First");
      // Chapter 2 has a scene under it, so the delete asks to confirm.
      await user.click(
        screen.getAllByRole("button", { name: "Delete chapter" })[2],
      );
      await user.click(screen.getByRole("button", { name: "Delete" }));
      act(() => tabRef.current?.save());

      expect(mutate).toHaveBeenCalledTimes(1);
      const saved = mutate.mock.calls[0][0].chapters;
      expect(saved).toHaveLength(2);
      expect(saved[0].title).toBe("Part One");
      expect(saved[0].children.map((c: { title: string }) => c.title)).toEqual([
        "First",
      ]);
      expect(saved[1].title).toBe("Epilogue");
    });
  },
);
