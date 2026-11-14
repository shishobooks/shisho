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
  let page: string | null;
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
    page = "1";
    ended = [];
    tracker = new ReadingSessionTracker({
      getPercentage: () => percentage,
      getPage: () => page,
      onSessionEnd: (session, reason) => ended.push({ session, reason }),
    });
  });

  afterEach(() => {
    tracker.stop();
  });

  it("ends a session after five idle minutes, at the last input", () => {
    tracker.start();
    vi.advanceTimersByTime(minutes(3));
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
          ended_at: at(minutes(3)),
          start_percentage: 0.1,
          end_percentage: 0.2,
          duration: 120,
        },
      },
    ]);
  });

  it("counts the time on each page, two minutes at most", () => {
    tracker.start();
    vi.advanceTimersByTime(30_000);
    page = "2";
    tracker.recordActivity();
    vi.advanceTimersByTime(minutes(4));
    page = "3";
    tracker.recordActivity();
    vi.advanceTimersByTime(minutes(1));
    setVisibility("hidden");

    expect(ended.map((e) => e.session)).toEqual([
      {
        started_at: at(0),
        ended_at: at(minutes(5.5)),
        start_percentage: 0.1,
        end_percentage: 0.1,
        duration: 30 + 120 + 60,
      },
    ]);
  });

  it("counts nothing for a page left within five seconds", () => {
    tracker.start();
    vi.advanceTimersByTime(4_000);
    page = "2";
    tracker.recordActivity();
    vi.advanceTimersByTime(4_999);
    page = "3";
    tracker.recordActivity();
    vi.advanceTimersByTime(5_000);
    page = "4";
    tracker.recordActivity();
    vi.advanceTimersByTime(minutes(1));
    tracker.stop();

    expect(ended[0].session.duration).toBe(5 + 60);
  });

  it("counts nothing before the reader knows its page", () => {
    page = null;
    tracker.start();
    vi.advanceTimersByTime(10_000);
    page = "1";
    tracker.recordActivity();
    vi.advanceTimersByTime(30_000);
    tracker.stop();

    expect(ended.map((e) => e.session)).toEqual([
      {
        started_at: at(10_000),
        ended_at: at(40_000),
        start_percentage: 0.1,
        end_percentage: 0.1,
        duration: 30,
      },
    ]);
  });

  it("records nothing for a reader that never showed a page", () => {
    page = null;
    tracker.start();
    vi.advanceTimersByTime(minutes(1));
    tracker.recordActivity();
    vi.advanceTimersByTime(READING_IDLE_MS);
    tracker.stop();
    expect(ended).toEqual([]);
  });

  it("counts input that stays on a page as the same visit", () => {
    tracker.start();
    for (let i = 0; i < 3; i++) {
      vi.advanceTimersByTime(minutes(1));
      tracker.recordActivity();
    }
    page = "2";
    tracker.recordActivity();
    vi.advanceTimersByTime(minutes(1));
    tracker.stop();

    expect(ended[0].session.duration).toBe(120 + 60);
  });

  it("ends an idle session two minutes into its last page, when that's after the last input", () => {
    tracker.start();
    vi.advanceTimersByTime(minutes(1));
    page = "2";
    tracker.recordActivity();
    vi.advanceTimersByTime(30_000);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Shift" }));
    vi.advanceTimersByTime(READING_IDLE_MS);

    expect(ended).toEqual([
      {
        reason: "idle",
        session: {
          started_at: at(0),
          ended_at: at(minutes(3)),
          start_percentage: 0.1,
          end_percentage: 0.1,
          duration: 60 + 120,
        },
      },
    ]);
  });

  it("records nothing for a session with no page read for five seconds", () => {
    tracker.start();
    vi.advanceTimersByTime(3_000);
    page = "2";
    tracker.recordActivity();
    vi.advanceTimersByTime(4_000);
    tracker.stop();
    expect(ended).toEqual([]);
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
        ended_at: at(minutes(2)),
        start_percentage: 0.1,
        end_percentage: 0.1,
        duration: 120,
      },
      {
        started_at: at(minutes(1) + READING_IDLE_MS + minutes(10)),
        ended_at: at(minutes(1) + READING_IDLE_MS + minutes(13)),
        start_percentage: 0.3,
        end_percentage: 0.3,
        duration: 120,
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
          duration: 120,
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
          duration: 120,
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

  describe("timing playback", () => {
    beforeEach(() => {
      tracker = new ReadingSessionTracker({
        timing: "playback",
        getPercentage: () => percentage,
        getPage: () => null,
        onSessionEnd: (session, reason) => ended.push({ session, reason }),
      });
      tracker.start();
    });

    it("has no session until playback starts", () => {
      window.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight" }));
      vi.advanceTimersByTime(minutes(1));
      tracker.setPlaying(false);
      tracker.stop();

      expect(ended).toEqual([]);
    });

    it("ends the session when playback pauses", () => {
      tracker.setPlaying(true);
      vi.advanceTimersByTime(minutes(3));
      percentage = 0.2;
      tracker.setPlaying(false);

      expect(ended).toEqual([
        {
          reason: "paused",
          session: {
            started_at: at(0),
            ended_at: at(minutes(3)),
            start_percentage: 0.1,
            end_percentage: 0.2,
          },
        },
      ]);
    });

    it("keeps counting in a hidden tab and past five minutes with no input", () => {
      tracker.setPlaying(true);
      setVisibility("hidden");
      vi.advanceTimersByTime(READING_IDLE_MS * 4);
      expect(ended).toEqual([]);

      setVisibility("visible");
      vi.advanceTimersByTime(minutes(1));
      tracker.setPlaying(false);

      expect(ended).toHaveLength(1);
      expect(ended[0].session.started_at).toBe(at(0));
      expect(ended[0].session.ended_at).toBe(
        at(READING_IDLE_MS * 4 + minutes(1)),
      );
    });

    it("delivers the session when the page closes while playing", () => {
      tracker.setPlaying(true);
      vi.advanceTimersByTime(minutes(2));
      window.dispatchEvent(new Event("pagehide"));

      expect(ended).toHaveLength(1);
      expect(ended[0].reason).toBe("close");
      expect(ended[0].session.ended_at).toBe(at(minutes(2)));
    });

    it("adds up one sitting across a short pause", () => {
      expect(tracker.listening()).toBeNull();

      tracker.setPlaying(true);
      vi.advanceTimersByTime(minutes(1));
      expect(tracker.listening()).toEqual({ since: at(0), seconds: 60 });

      tracker.setPlaying(false);
      expect(tracker.listening()).toEqual({ since: at(0), seconds: 60 });

      vi.advanceTimersByTime(minutes(4));
      tracker.setPlaying(true);
      vi.advanceTimersByTime(minutes(1));
      expect(tracker.listening()).toEqual({ since: at(0), seconds: 120 });
    });

    it("stops reporting a sitting over a day old, which has nothing left to decide", () => {
      tracker.setPlaying(true);
      vi.advanceTimersByTime(minutes(24 * 60));
      expect(tracker.listening()).not.toBeNull();

      vi.advanceTimersByTime(1);
      expect(tracker.listening()).toBeNull();
    });

    it("starts a new sitting after a pause longer than the idle limit", () => {
      tracker.setPlaying(true);
      vi.advanceTimersByTime(minutes(1));
      tracker.setPlaying(false);

      vi.advanceTimersByTime(READING_IDLE_MS + 1);
      tracker.setPlaying(true);
      vi.advanceTimersByTime(minutes(1));

      expect(tracker.listening()).toEqual({
        since: at(minutes(1) + READING_IDLE_MS + 1),
        seconds: 60,
      });
    });

    it("starts a new session each time playback starts", () => {
      tracker.setPlaying(true);
      vi.advanceTimersByTime(minutes(1));
      tracker.setPlaying(false);
      vi.advanceTimersByTime(minutes(10));
      tracker.setPlaying(true);
      vi.advanceTimersByTime(minutes(1));
      tracker.stop();

      expect(ended.map((end) => end.session.started_at)).toEqual([
        at(0),
        at(minutes(11)),
      ]);
    });
  });
});
