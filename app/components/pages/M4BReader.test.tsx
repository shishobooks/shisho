import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Book, File } from "@/types";

import M4BReader from "./M4BReader";

// jsdom does not implement the HTMLMediaElement playback pipeline. Stub the
// methods so the component can call them, and let tests drive currentTime /
// duration / paused manually plus dispatch the media events they assert on.
let playSpy: ReturnType<typeof vi.spyOn>;
let pauseSpy: ReturnType<typeof vi.spyOn>;

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
});
