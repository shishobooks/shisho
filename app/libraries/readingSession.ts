import type { SessionPayload } from "@/types";

// A Reading Session ends after this long with no input: a tab left open
// overnight isn't eight hours of reading.
export const READING_IDLE_MS = 5 * 60 * 1000;

// Why a session ended. "close" is the page unloading, so whoever delivers
// it needs a request that survives unload.
export type SessionEndReason = "idle" | "hidden" | "close" | "stop";

interface ReadingSessionTrackerOptions {
  // How far through the File the reader is now, or null before it knows.
  getPercentage: () => number | null;
  onSessionEnd: (session: SessionPayload, reason: SessionEndReason) => void;
}

interface OpenSession {
  startedAt: number;
  startPercentage: number | null;
  lastActivityAt: number;
}

// Input on the page. The readers forward input inside their own frames
// (foliate's book iframe) through recordActivity.
export const ACTIVITY_EVENTS = [
  "pointerdown",
  "keydown",
  "wheel",
  "touchstart",
];

// Times Reading Sessions for one reader, shared by every web reader. Time
// counts while the page is visible and there was input in the last five
// minutes. A session ends after five idle minutes (at its last input), when
// the page is hidden, when it closes, or on stop.
export class ReadingSessionTracker {
  private readonly options: ReadingSessionTrackerOptions;
  private session: OpenSession | null = null;
  private idleTimer: ReturnType<typeof setTimeout> | null = null;
  private started = false;

  constructor(options: ReadingSessionTrackerOptions) {
    this.options = options;
  }

  // Starts listening. Opening the reader counts as input.
  start() {
    if (this.started) return;
    this.started = true;
    for (const name of ACTIVITY_EVENTS) {
      window.addEventListener(name, this.recordActivity, { passive: true });
    }
    document.addEventListener("visibilitychange", this.handleVisibility);
    window.addEventListener("pagehide", this.handlePageHide);
    this.recordActivity();
  }

  // Ends the open session and stops listening.
  stop() {
    if (!this.started) return;
    this.started = false;
    for (const name of ACTIVITY_EVENTS) {
      window.removeEventListener(name, this.recordActivity);
    }
    document.removeEventListener("visibilitychange", this.handleVisibility);
    window.removeEventListener("pagehide", this.handlePageHide);
    this.end("stop", Date.now());
  }

  recordActivity = () => {
    if (!this.started || document.visibilityState !== "visible") return;
    const now = Date.now();
    if (!this.session) {
      this.session = {
        startedAt: now,
        startPercentage: null,
        lastActivityAt: now,
      };
    }
    this.session.startPercentage ??= this.options.getPercentage();
    this.session.lastActivityAt = now;
    if (this.idleTimer) clearTimeout(this.idleTimer);
    this.idleTimer = setTimeout(() => {
      this.end("idle", this.session?.lastActivityAt ?? Date.now());
    }, READING_IDLE_MS);
  };

  private handleVisibility = () => {
    if (document.visibilityState === "visible") this.recordActivity();
    else this.end("hidden", Date.now());
  };

  private handlePageHide = () => {
    this.end("close", Date.now());
  };

  private end(reason: SessionEndReason, endedAt: number) {
    if (this.idleTimer) {
      clearTimeout(this.idleTimer);
      this.idleTimer = null;
    }
    const session = this.session;
    this.session = null;
    if (!session || endedAt <= session.startedAt) return;
    const endPercentage =
      this.options.getPercentage() ?? session.startPercentage ?? 0;
    this.options.onSessionEnd(
      {
        started_at: new Date(session.startedAt).toISOString(),
        ended_at: new Date(endedAt).toISOString(),
        start_percentage: session.startPercentage ?? endPercentage,
        end_percentage: endPercentage,
      },
      reason,
    );
  }
}
