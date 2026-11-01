import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  READING_IDLE_MS,
  ReadingSessionTracker,
  type SessionEndReason,
} from "@/libraries/readingSession";
import type { SessionPayload } from "@/types";

const START = new Date("2026-10-15T12:00:00Z");

let visibility: DocumentVisibilityState = "visible";
const setVisibility = (next: DocumentVisibilityState) => {
  visibility = next;
  document.dispatchEvent(new Event("visibilitychange"));
};

const minutes = (n: number) => n * 60_000;
const at = (offsetMs: number) =>
  new Date(START.getTime() + offsetMs).toISOString();

describe("ReadingSessionTracker", () => {
  let percentage: number | null;
  let ended: Array<{ session: SessionPayload; reason: SessionEndReason }>;
  let tracker: ReadingSessionTracker;

  beforeEach(() => {
    vi.setSystemTime(START);
    visibility = "visible";
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => visibility,
    });
    percentage = 0.1;
    ended = [];
    tracker = new ReadingSessionTracker({
      getPercentage: () => percentage,
      onSessionEnd: (session, reason) => ended.push({ session, reason }),
    });
  });

  afterEach(() => {
    tracker.stop();
  });

  it("ends a session after five idle minutes, at the last input", () => {
    tracker.start();
    vi.advanceTimersByTime(minutes(2));
    percentage = 0.2;
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight" }));

    vi.advanceTimersByTime(READING_IDLE_MS - 1);
    expect(ended).toEqual([]);
    vi.advanceTimersByTime(1);

    expect(ended).toEqual([
      {
        reason: "idle",
        session: {
          started_at: at(0),
          ended_at: at(minutes(2)),
          start_percentage: 0.1,
          end_percentage: 0.2,
        },
      },
    ]);
  });

  it("starts a new session on the next input after going idle", () => {
    tracker.start();
    vi.advanceTimersByTime(minutes(1));
    tracker.recordActivity();
    vi.advanceTimersByTime(READING_IDLE_MS + minutes(10));
    percentage = 0.3;
    window.dispatchEvent(new Event("pointerdown"));
    vi.advanceTimersByTime(minutes(3));
    tracker.recordActivity();
    vi.advanceTimersByTime(READING_IDLE_MS);

    expect(ended.map((e) => e.session)).toEqual([
      {
        started_at: at(0),
        ended_at: at(minutes(1)),
        start_percentage: 0.1,
        end_percentage: 0.1,
      },
      {
        started_at: at(minutes(1) + READING_IDLE_MS + minutes(10)),
        ended_at: at(minutes(1) + READING_IDLE_MS + minutes(13)),
        start_percentage: 0.3,
        end_percentage: 0.3,
      },
    ]);
  });

  it("ends a session when the page is hidden, and counts nothing while hidden", () => {
    tracker.start();
    vi.advanceTimersByTime(minutes(3));
    percentage = 0.15;
    setVisibility("hidden");

    expect(ended).toEqual([
      {
        reason: "hidden",
        session: {
          started_at: at(0),
          ended_at: at(minutes(3)),
          start_percentage: 0.1,
          end_percentage: 0.15,
        },
      },
    ]);

    // Input reaching a hidden page isn't reading.
    tracker.recordActivity();
    vi.advanceTimersByTime(READING_IDLE_MS * 2);
    expect(ended).toHaveLength(1);

    // Coming back starts a new session.
    setVisibility("visible");
    vi.advanceTimersByTime(minutes(1));
    tracker.recordActivity();
    vi.advanceTimersByTime(READING_IDLE_MS);
    expect(ended).toHaveLength(2);
    expect(ended[1].session.started_at).toBe(
      at(minutes(3) + READING_IDLE_MS * 2),
    );
  });

  it("delivers the session when the page closes", () => {
    tracker.start();
    vi.advanceTimersByTime(minutes(4));
    window.dispatchEvent(new Event("pagehide"));

    expect(ended).toEqual([
      {
        reason: "close",
        session: {
          started_at: at(0),
          ended_at: at(minutes(4)),
          start_percentage: 0.1,
          end_percentage: 0.1,
        },
      },
    ]);
  });

  it("ends the session on stop and stops listening", () => {
    tracker.start();
    vi.advanceTimersByTime(minutes(1));
    tracker.stop();
    expect(ended.map((e) => e.reason)).toEqual(["stop"]);

    window.dispatchEvent(new Event("pointerdown"));
    vi.advanceTimersByTime(READING_IDLE_MS * 2);
    window.dispatchEvent(new Event("pagehide"));
    expect(ended).toHaveLength(1);
  });

  it("records nothing for a session with no time in it", () => {
    tracker.start();
    setVisibility("hidden");
    expect(ended).toEqual([]);
  });

  it("takes the start percentage from the first one known", () => {
    percentage = null;
    tracker.start();
    vi.advanceTimersByTime(minutes(1));
    percentage = 0.4;
    tracker.recordActivity();
    vi.advanceTimersByTime(minutes(1));
    percentage = 0.45;
    setVisibility("hidden");

    expect(ended[0].session.start_percentage).toBe(0.4);
    expect(ended[0].session.end_percentage).toBe(0.45);
  });

  it("starts no session while the page is hidden", () => {
    visibility = "hidden";
    tracker.start();
    vi.advanceTimersByTime(minutes(2));
    window.dispatchEvent(new Event("pagehide"));
    expect(ended).toEqual([]);
  });
});
