import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import type { Book, File } from "@/types";

import M4BReader from "./M4BReader";

// The player reads/writes the persisted playback speed through the
// user-settings hooks; mock them so tests control the stored speed and can
// assert on the persistence call without a QueryClient or network.
vi.mock("@/hooks/queries/settings", () => ({
  useUserSettings: vi.fn(),
  useUpdateUserSettings: vi.fn(),
}));

// jsdom does not implement the HTMLMediaElement playback pipeline. Stub the
// methods so the component can call them, and let tests drive currentTime /
// duration / paused manually plus dispatch the media events they assert on.
let playSpy: ReturnType<typeof vi.spyOn>;
let pauseSpy: ReturnType<typeof vi.spyOn>;

let updateSettingsMutate: ReturnType<typeof vi.fn>;

beforeEach(() => {
  playSpy = vi
    .spyOn(window.HTMLMediaElement.prototype, "play")
    .mockResolvedValue(undefined);
  pauseSpy = vi
    .spyOn(window.HTMLMediaElement.prototype, "pause")
    .mockImplementation(() => {});
  vi.spyOn(window.HTMLMediaElement.prototype, "load").mockImplementation(
    () => {},
  );

  updateSettingsMutate = vi.fn();
  vi.mocked(useUserSettings).mockReturnValue({
    data: undefined,
    isLoading: false,
  } as never);
  vi.mocked(useUpdateUserSettings).mockReturnValue({
    mutate: updateSettingsMutate,
  } as never);
});

afterEach(() => {
  vi.restoreAllMocks();
  document.title = "";
});

const narratorPerson = { id: 9, name: "Jane Narrator" } as never;
const authorPerson = { id: 5, name: "Sam Author" } as never;

const file = {
  id: 42,
  book_id: 7,
  file_type: "m4b",
  filepath: "/lib/book.m4b",
  audiobook_duration_seconds: 3600,
  narrators: [{ id: 1, file_id: 42, person_id: 9, person: narratorPerson }],
} as unknown as File;

// A file with three chapters spanning a 3600s book: 0–1200, 1200–2400, 2400–end.
// Chapter starts are in MILLISECONDS on the model.
const fileWithChapters = {
  ...file,
  chapters: [
    { id: 1, title: "Chapter One", start_timestamp_ms: 0, sort_order: 0 },
    {
      id: 2,
      title: "Chapter Two",
      start_timestamp_ms: 1200000,
      sort_order: 1,
    },
    {
      id: 3,
      title: "Chapter Three",
      start_timestamp_ms: 2400000,
      sort_order: 2,
    },
  ],
} as unknown as File;

const book = {
  id: 7,
  title: "The Test Audiobook",
  cover_cache_key: "7-123",
  authors: [{ id: 2, book_id: 7, person_id: 5, person: authorPerson }],
  files: [file],
} as unknown as Book;

const renderReader = (props?: { book?: Book; file?: File }) =>
  render(
    <MemoryRouter>
      <M4BReader
        book={props?.book ?? book}
        file={props?.file ?? file}
        libraryId="1"
      />
    </MemoryRouter>,
  );

const getAudio = () =>
  document.querySelector("audio") as HTMLAudioElement & {
    currentTime: number;
    duration: number;
  };

describe("M4BReader", () => {
  it("renders an audio element pointed at the stream endpoint", () => {
    renderReader();
    const audio = getAudio();
    expect(audio).toBeInTheDocument();
    expect(audio.getAttribute("src")).toBe("/api/books/files/42/stream");
  });

  it("does not autoplay on mount", () => {
    renderReader();
    expect(playSpy).not.toHaveBeenCalled();
    // Play button should be shown (paused state), not a pause button.
    expect(screen.getByRole("button", { name: /play/i })).toBeInTheDocument();
  });

  it("displays the book title, author, and narrator", () => {
    renderReader();
    expect(
      screen.getByRole("heading", { name: "The Test Audiobook" }),
    ).toBeInTheDocument();
    expect(screen.getByText(/Sam Author/)).toBeInTheDocument();
    expect(screen.getByText(/Jane Narrator/)).toBeInTheDocument();
  });

  it("renders the book cover with a cache-busted URL", () => {
    renderReader();
    const img = screen.getByRole("img") as HTMLImageElement;
    expect(img.getAttribute("src")).toBe("/api/books/7/cover?v=7-123");
  });

  it("sets the browser tab title to the book title", () => {
    renderReader();
    expect(document.title).toContain("The Test Audiobook");
  });

  it("uses the file's audiobook_duration_seconds as the total time", () => {
    renderReader();
    // 3600s => 1:00:00
    expect(screen.getByText("1:00:00")).toBeInTheDocument();
  });

  it("plays when the play button is clicked and shows a pause control", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderReader();
    await user.click(screen.getByRole("button", { name: /play/i }));
    expect(playSpy).toHaveBeenCalled();

    // Simulate the audio element firing its play event.
    fireEvent.play(getAudio());
    expect(screen.getByRole("button", { name: /pause/i })).toBeInTheDocument();
  });

  it("pauses when the pause button is clicked while playing", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    renderReader();
    await user.click(screen.getByRole("button", { name: /play/i }));
    fireEvent.play(getAudio());
    await user.click(screen.getByRole("button", { name: /pause/i }));
    expect(pauseSpy).toHaveBeenCalled();
  });

  it("advances the elapsed time label as the audio reports timeupdate", () => {
    renderReader();
    const audio = getAudio();
    audio.currentTime = 65;
    fireEvent.timeUpdate(audio);
    expect(screen.getByText("1:05")).toBeInTheDocument();
  });

  it("seeks the audio when the seek bar value is committed", () => {
    renderReader();
    const audio = getAudio();
    // Radix sliders expose role=slider; simulate a keyboard seek which fires
    // value change + commit. Use the component's exposed test affordance: a
    // direct currentTime set via the commit path is asserted through the
    // slider's onValueCommit. We drive it via the slider thumb.
    const slider = screen.getByRole("slider", { name: /seek/i });
    slider.focus();
    // ArrowRight on a Radix slider increments by step (1s) and commits.
    fireEvent.keyDown(slider, { key: "ArrowRight" });
    expect(audio.currentTime).toBeGreaterThan(0);
  });

  it("stays on the player and pauses at end of file", () => {
    renderReader();
    const audio = getAudio();
    audio.currentTime = 3600;
    fireEvent.play(audio);
    fireEvent.ended(audio);
    // After ended, the play (not pause) control is shown again.
    expect(screen.getByRole("button", { name: /play/i })).toBeInTheDocument();
  });

  it("toggles play/pause with the space key and prevents page scroll", () => {
    renderReader();
    // Initially paused: space should trigger play.
    const ev = new KeyboardEvent("keydown", {
      key: " ",
      bubbles: true,
      cancelable: true,
    });
    const prevented = !document.dispatchEvent(ev);
    expect(playSpy).toHaveBeenCalled();
    expect(prevented).toBe(true);
  });

  it("falls back to the audio element duration when no metadata duration", () => {
    const fileNoDuration = {
      ...file,
      audiobook_duration_seconds: undefined,
    } as unknown as File;
    renderReader({ file: fileNoDuration });
    const audio = getAudio();
    Object.defineProperty(audio, "duration", {
      value: 125,
      configurable: true,
    });
    fireEvent.loadedMetadata(audio);
    // 125s => 2:05
    expect(screen.getByText("2:05")).toBeInTheDocument();
  });

  describe("playback speed", () => {
    it("defaults to 1x when no speed has been persisted", () => {
      renderReader();
      const combobox = screen.getByRole("combobox", {
        name: /playback speed/i,
      });
      expect(combobox).toHaveTextContent("1x");
      expect(getAudio().playbackRate).toBe(1);
    });

    it("applies the persisted speed from user settings to the audio element", () => {
      vi.mocked(useUserSettings).mockReturnValue({
        data: { viewer_playback_speed: 1.5 },
        isLoading: false,
      } as never);
      renderReader();
      expect(getAudio().playbackRate).toBe(1.5);
      expect(
        screen.getByRole("combobox", { name: /playback speed/i }),
      ).toHaveTextContent("1.5x");
    });

    it("re-applies the speed after the media load resets the rate", () => {
      vi.mocked(useUserSettings).mockReturnValue({
        data: { viewer_playback_speed: 2.5 },
        isLoading: false,
      } as never);
      renderReader();
      const audio = getAudio();

      // The HTML media load algorithm resets playbackRate to the default
      // (1) as part of loading the resource, which can land after the
      // component applied the persisted speed. Simulate the reset, then the
      // metadata arriving.
      audio.playbackRate = 1;
      fireEvent.loadedMetadata(audio);

      expect(audio.playbackRate).toBe(2.5);
    });

    it("changes the playback rate immediately when a speed is chosen", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader();

      await user.click(
        screen.getByRole("combobox", { name: /playback speed/i }),
      );
      await user.click(await screen.findByRole("option", { name: "2x" }));

      expect(getAudio().playbackRate).toBe(2);
    });

    it("persists the chosen speed through the user-settings mutation", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader();

      await user.click(
        screen.getByRole("combobox", { name: /playback speed/i }),
      );
      await user.click(await screen.findByRole("option", { name: "0.75x" }));

      expect(updateSettingsMutate).toHaveBeenCalledWith({
        viewer_playback_speed: 0.75,
      });
    });

    it("offers every discrete step from 0.5x to 3x", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader();

      await user.click(
        screen.getByRole("combobox", { name: /playback speed/i }),
      );
      const options = await screen.findAllByRole("option");
      expect(options.map((o) => o.textContent)).toEqual([
        "0.5x",
        "0.75x",
        "1x",
        "1.25x",
        "1.5x",
        "1.75x",
        "2x",
        "2.5x",
        "3x",
      ]);
    });
  });

  describe("chapter navigation", () => {
    it("renders a chapter dropdown that jumps to the selected chapter's start", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader({ file: fileWithChapters });
      const audio = getAudio();

      await user.click(screen.getByRole("combobox", { name: /chapter/i }));
      await user.click(
        await screen.findByRole("option", { name: /chapter three/i }),
      );
      // Chapter Three starts at 2400000ms => 2400s.
      expect(audio.currentTime).toBe(2400);
    });

    it("shows the current chapter title and updates it as playback crosses a boundary", () => {
      renderReader({ file: fileWithChapters });
      const audio = getAudio();
      const combobox = screen.getByRole("combobox", { name: /chapter/i });

      // Start of the file: first chapter is the displayed/selected chapter.
      expect(combobox).toHaveTextContent("Chapter One");

      // Advance past the second chapter boundary (1200s).
      audio.currentTime = 1300;
      fireEvent.timeUpdate(audio);
      expect(combobox).toHaveTextContent("Chapter Two");
      expect(combobox).not.toHaveTextContent("Chapter One");
    });

    it("renders chapter markers along the seek bar at the correct positions", () => {
      const { container } = renderReader({ file: fileWithChapters });
      // The first chapter starts at 0 (no marker); the two later chapters get
      // markers positioned by start/duration.
      const markers = container.querySelectorAll(
        '[aria-hidden="true"][style*="left"]',
      );
      const lefts = Array.from(markers).map(
        (m) => (m as HTMLElement).style.left,
      );
      // 1200/3600 => 33.33%, 2400/3600 => 66.66%
      expect(lefts).toHaveLength(2);
      expect(lefts[0]).toMatch(/^33\.33/);
      expect(lefts[1]).toMatch(/^66\.66/);
    });

    it("advances to the next chapter with the next button", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader({ file: fileWithChapters });
      const audio = getAudio();

      await user.click(screen.getByRole("button", { name: /next chapter/i }));
      // From 0 (chapter one), next chapter starts at 1200s.
      expect(audio.currentTime).toBe(1200);
    });

    it("restarts the current chapter with previous when more than ~5s in", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader({ file: fileWithChapters });
      const audio = getAudio();

      // Move to 1300s (well into chapter two, which starts at 1200s).
      audio.currentTime = 1300;
      fireEvent.timeUpdate(audio);
      await user.click(
        screen.getByRole("button", { name: /previous chapter/i }),
      );
      // More than 5s in => restart current chapter (1200s).
      expect(audio.currentTime).toBe(1200);
    });

    it("goes to the prior chapter with previous when within ~5s of the start", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader({ file: fileWithChapters });
      const audio = getAudio();

      // Move to 1203s (just 3s into chapter two).
      audio.currentTime = 1203;
      fireEvent.timeUpdate(audio);
      await user.click(
        screen.getByRole("button", { name: /previous chapter/i }),
      );
      // Within 5s => go to prior chapter (chapter one, start 0).
      expect(audio.currentTime).toBe(0);
    });

    it("skips forward and back by 30 seconds with the skip buttons", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderReader({ file: fileWithChapters });
      const audio = getAudio();

      audio.currentTime = 100;
      fireEvent.timeUpdate(audio);

      await user.click(
        screen.getByRole("button", { name: /skip forward 30 seconds/i }),
      );
      expect(audio.currentTime).toBe(130);

      await user.click(
        screen.getByRole("button", { name: /skip back 30 seconds/i }),
      );
      expect(audio.currentTime).toBe(100);
    });

    it("skips with the left and right arrow keys", () => {
      renderReader({ file: fileWithChapters });
      const audio = getAudio();
      audio.currentTime = 100;
      fireEvent.timeUpdate(audio);

      const right = new KeyboardEvent("keydown", {
        key: "ArrowRight",
        bubbles: true,
        cancelable: true,
      });
      const rightPrevented = !document.dispatchEvent(right);
      expect(audio.currentTime).toBe(130);
      expect(rightPrevented).toBe(true);

      const left = new KeyboardEvent("keydown", {
        key: "ArrowLeft",
        bubbles: true,
        cancelable: true,
      });
      const leftPrevented = !document.dispatchEvent(left);
      expect(audio.currentTime).toBe(100);
      expect(leftPrevented).toBe(true);
    });

    it("clamps arrow-key skips to the start and end of the file", () => {
      renderReader({ file: fileWithChapters });
      const audio = getAudio();

      // Near the start: skip back clamps to 0.
      audio.currentTime = 10;
      fireEvent.timeUpdate(audio);
      document.dispatchEvent(
        new KeyboardEvent("keydown", { key: "ArrowLeft", bubbles: true }),
      );
      expect(audio.currentTime).toBe(0);

      // Near the end: skip forward clamps to the duration (3600s).
      audio.currentTime = 3590;
      fireEvent.timeUpdate(audio);
      document.dispatchEvent(
        new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }),
      );
      expect(audio.currentTime).toBe(3600);
    });

    it("renders the player with chapter navigation absent for a file with no chapters", () => {
      renderReader({ file });
      // No chapter controls when there are no chapters.
      expect(
        screen.queryByRole("combobox", { name: /chapter/i }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /next chapter/i }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /previous chapter/i }),
      ).not.toBeInTheDocument();
      // The player itself still works.
      expect(screen.getByRole("button", { name: /play/i })).toBeInTheDocument();
      // Plain skip controls remain available even without chapters.
      expect(
        screen.getByRole("button", { name: /skip forward 30 seconds/i }),
      ).toBeInTheDocument();
    });
  });
});
