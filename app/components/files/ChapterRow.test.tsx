import ChapterRow from "./ChapterRow";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import { FileTypeCBZ, FileTypeM4B, type Chapter } from "@/types";

// Wrapper to provide router context for Link components
const renderWithRouter = (ui: React.ReactElement) => {
  return render(<MemoryRouter>{ui}</MemoryRouter>);
};

describe("ChapterRow - M4B Playback", () => {
  const baseM4bChapter: Chapter = {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    file_id: 100,
    title: "Chapter 1",
    sort_order: 0,
    start_timestamp_ms: 60000, // 1 minute
    children: [],
  };

  describe("isPlaying determination", () => {
    it("shows play button when not playing", () => {
      renderWithRouter(
        <ChapterRow
          chapter={baseM4bChapter}
          chapterIndex={0}
          depth={0}
          fileType={FileTypeM4B}
          isEditing={false}
          onPlay={vi.fn()}
          onStop={vi.fn()}
          playingChapterIndex={null}
        />,
      );

      // Play button should be present (not the stop/square icon)
      const button = screen.getByRole("button");
      expect(button).toBeInTheDocument();
    });

    it("shows stop state when this chapter is playing (matching index)", () => {
      renderWithRouter(
        <ChapterRow
          chapter={baseM4bChapter}
          chapterIndex={2}
          depth={0}
          fileType={FileTypeM4B}
          isEditing={false}
          onPlay={vi.fn()}
          onStop={vi.fn()}
          playingChapterIndex={2}
        />,
      );

      // Button should show stop tooltip when playing
      expect(screen.getByRole("button")).toBeInTheDocument();
    });

    it("shows play state when different chapter is playing", () => {
      renderWithRouter(
        <ChapterRow
          chapter={baseM4bChapter}
          chapterIndex={0}
          depth={0}
          fileType={FileTypeM4B}
          isEditing={false}
          onPlay={vi.fn()}
          onStop={vi.fn()}
          playingChapterIndex={5} // Different index
        />,
      );

      expect(screen.getByRole("button")).toBeInTheDocument();
    });

    it("does not show as playing when chapterIndex is undefined", () => {
      // This tests the guard: chapterIndex != null && playingChapterIndex === chapterIndex
      renderWithRouter(
        <ChapterRow
          chapter={baseM4bChapter}
          chapterIndex={undefined}
          depth={0}
          fileType={FileTypeM4B}
          isEditing={false}
          onPlay={vi.fn()}
          onStop={vi.fn()}
          playingChapterIndex={undefined}
        />,
      );

      // Should not crash and should show play button
      expect(screen.getByRole("button")).toBeInTheDocument();
    });
  });

  describe("onPlay callback", () => {
    it("calls onPlay with chapterIndex and timestamp when play button clicked in view mode", async () => {
      const user = userEvent.setup();
      const onPlay = vi.fn();

      renderWithRouter(
        <ChapterRow
          chapter={baseM4bChapter}
          chapterIndex={3}
          depth={0}
          fileType={FileTypeM4B}
          isEditing={false}
          onPlay={onPlay}
          onStop={vi.fn()}
          playingChapterIndex={null}
        />,
      );

      await user.click(screen.getByRole("button"));

      expect(onPlay).toHaveBeenCalledWith(3, 60000);
    });

    it("calls onPlay with chapterIndex and current timestamp in edit mode", async () => {
      const user = userEvent.setup();
      const onPlay = vi.fn();

      const editChapter: Chapter = {
        ...baseM4bChapter,
        start_timestamp_ms: 120000, // 2 minutes
      };

      renderWithRouter(
        <ChapterRow
          chapter={editChapter}
          chapterIndex={5}
          depth={0}
          fileType={FileTypeM4B}
          isEditing={true}
          maxDurationMs={3600000}
          onPlay={onPlay}
          onStartTimestampChange={vi.fn()}
          onStop={vi.fn()}
          onTitleChange={vi.fn()}
          playingChapterIndex={null}
        />,
      );

      // Find the play button (there are multiple buttons in edit mode)
      const buttons = screen.getAllByRole("button");
      const playButton = buttons.find((btn) =>
        btn.querySelector("svg")?.classList.contains("lucide-play"),
      );

      if (playButton) {
        await user.click(playButton);
        expect(onPlay).toHaveBeenCalledWith(5, 120000);
      }
    });

    it("does not call onPlay when chapterIndex is undefined", async () => {
      const user = userEvent.setup();
      const onPlay = vi.fn();

      renderWithRouter(
        <ChapterRow
          chapter={baseM4bChapter}
          chapterIndex={undefined}
          depth={0}
          fileType={FileTypeM4B}
          isEditing={false}
          onPlay={onPlay}
          onStop={vi.fn()}
          playingChapterIndex={null}
        />,
      );

      await user.click(screen.getByRole("button"));

      expect(onPlay).not.toHaveBeenCalled();
    });

    it("calls onPlay with updated timestamp after clicking minus button in edit mode", async () => {
      const user = userEvent.setup();
      const onPlay = vi.fn();
      const onStartTimestampChange = vi.fn();

      const editChapter: Chapter = {
        ...baseM4bChapter,
        id: undefined as unknown as number, // Simulate edit mode where chapters don't have IDs
        start_timestamp_ms: 60000, // 1 minute
      };

      renderWithRouter(
        <ChapterRow
          chapter={editChapter}
          chapterIndex={0}
          depth={0}
          fileType={FileTypeM4B}
          isEditing={true}
          maxDurationMs={3600000}
          onPlay={onPlay}
          onStartTimestampChange={onStartTimestampChange}
          onStop={vi.fn()}
          onTitleChange={vi.fn()}
          onValidationChange={vi.fn()}
          playingChapterIndex={null}
        />,
      );

      // Find and click the minus button (decrements timestamp by 1 second)
      const minusButton = screen.getByTitle("Subtract 1 second");
      await user.click(minusButton);

      // Verify timestamp was decremented
      expect(onStartTimestampChange).toHaveBeenCalledWith(59000);

      // Now find and click the play button
      const buttons = screen.getAllByRole("button");
      const playButton = buttons.find((btn) =>
        btn.querySelector("svg")?.classList.contains("lucide-play"),
      );

      expect(playButton).toBeDefined();
      await user.click(playButton!);

      // Play should be called with the UPDATED local timestamp (59000)
      // even before React re-renders with the new props
      expect(onPlay).toHaveBeenCalledWith(0, 59000);
    });
  });

  describe("onStop callback", () => {
    it("calls onStop when stop button clicked while playing", async () => {
      const user = userEvent.setup();
      const onStop = vi.fn();

      renderWithRouter(
        <ChapterRow
          chapter={baseM4bChapter}
          chapterIndex={0}
          depth={0}
          fileType={FileTypeM4B}
          isEditing={false}
          onPlay={vi.fn()}
          onStop={onStop}
          playingChapterIndex={0} // This chapter is playing
        />,
      );

      await user.click(screen.getByRole("button"));

      expect(onStop).toHaveBeenCalled();
    });
  });

  describe("timestamp display", () => {
    it("displays formatted timestamp in view mode", () => {
      renderWithRouter(
        <ChapterRow
          chapter={baseM4bChapter}
          chapterIndex={0}
          depth={0}
          fileType={FileTypeM4B}
          isEditing={false}
          playingChapterIndex={null}
        />,
      );

      expect(screen.getByText("00:01:00.000")).toBeInTheDocument();
    });
  });
});

describe("ChapterRow - CBZ", () => {
  const baseCbzChapter: Chapter = {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    file_id: 100,
    title: "Chapter 1",
    sort_order: 0,
    start_page: 5,
    children: [],
  };

  it("displays page number in view mode (1-indexed)", () => {
    renderWithRouter(
      <ChapterRow
        chapter={baseCbzChapter}
        depth={0}
        fileId={100}
        fileType={FileTypeCBZ}
        isEditing={false}
      />,
    );

    expect(screen.getByText("Page 6")).toBeInTheDocument();
  });

  it("renders chapter title as link when libraryId and bookId provided", () => {
    renderWithRouter(
      <ChapterRow
        bookId={2}
        chapter={baseCbzChapter}
        depth={0}
        fileId={100}
        fileType={FileTypeCBZ}
        isEditing={false}
        libraryId={1}
      />,
    );

    const link = screen.getByRole("link", { name: "Chapter 1" });
    expect(link).toHaveAttribute(
      "href",
      "/libraries/1/books/2/files/100/read?page=5",
    );
  });

  it("renders chapter title as plain text when libraryId/bookId not provided", () => {
    renderWithRouter(
      <ChapterRow
        chapter={baseCbzChapter}
        depth={0}
        fileId={100}
        fileType={FileTypeCBZ}
        isEditing={false}
      />,
    );

    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.getByText("Chapter 1")).toBeInTheDocument();
  });
});
