import type { SessionPayload } from "@/types";

// A Reading Session ends after this long with no input: a tab left open
// overnight isn't eight hours of reading.
export const READING_IDLE_MS = 5 * 60 * 1000;

// Time on one page counts from 5 seconds, so pages flipped past add nothing,
// up to 2 minutes, so a page left open doesn't count as reading it. These are
// KOReader's defaults, so time from the two readers adds up the same way.
const MIN_PAGE_MS = 5 * 1000;
const MAX_PAGE_MS = 2 * 60 * 1000;

// A sitting's minute of listening has long been decided by this age, and the
// server refuses one older (maxSessionLength in pkg/reading/handlers.go).
const MAX_SITTING_MS = 24 * 60 * 60 * 1000;

// Why a session ended. "close" is the page unloading, so whoever delivers
// it needs a request that survives unload.
export type SessionEndReason = "idle" | "hidden" | "close" | "stop" | "paused";

// What counts as time spent: "activity" is a visible page with recent input,
// for reading; "playback" is audio playing, whatever the tab is doing.
export type SessionTiming = "activity" | "playback";

interface ReadingSessionTrackerOptions {
  timing?: SessionTiming;
  // How far through the File the reader is now, or null before it knows.
  getPercentage: () => number | null;
  // Which page or screen the reader shows now, or null before it shows one.
  // Time counts per page, so a change here on input is a page turn, and
  // nothing counts while it's null.
  getPage: () => string | null;
  onSessionEnd: (session: SessionPayload, reason: SessionEndReason) => void;
}

interface OpenSession {
  startedAt: number;
  startPercentage: number | null;
  lastActivityAt: number;
  // The page showing since pageSince, and the time counted on earlier pages.
  page: string | null;
  pageSince: number;
  countedMs: number;
}

// The time a visit to one page counts for.
const pageTime = (ms: number) =>
  ms < MIN_PAGE_MS ? 0 : Math.min(ms, MAX_PAGE_MS);

interface Sitting {
  startedAt: number;
  playedMs: number;
  pausedAt: number;
}

// Input on the page. The readers forward input inside their own frames
// (foliate's book iframe) through recordActivity.
export const ACTIVITY_EVENTS = [
  "pointerdown",
  "keydown",
  "wheel",
  "touchstart",
];

// Times Reading Sessions for one reader, shared by every web reader. A
// session runs while the page is visible and there was input in the last five
// minutes. It ends after five idle minutes (at its last input, or two minutes
// into the page showing when that's later), when the page is hidden, when it
// closes, or on stop. Its time is the time on each page, from 5 seconds up to
// 2 minutes, counted from when the first page shows, and a session with none
// is left out. With "playback" timing,
// time counts from setPlaying(true) until setPlaying(false), the page closing,
// or stop, in a hidden tab too, and input means nothing.
export class ReadingSessionTracker {
  private readonly options: ReadingSessionTrackerOptions;
  private session: OpenSession | null = null;
  // Playing since the player opened, across pauses shorter than the idle limit.
  // Each pause ends a session, but one sitting's play is what decides that this
  // listening is reading.
  private sitting: Sitting | null = null;
  private idleTimer: ReturnType<typeof setTimeout> | null = null;
  private started = false;

  constructor(options: ReadingSessionTrackerOptions) {
    this.options = options;
  }

  // Starts listening. Opening the reader counts as input.
  start() {
    if (this.started) return;
    this.started = true;
    window.addEventListener("pagehide", this.handlePageHide);
    if (this.options.timing === "playback") return;
    for (const name of ACTIVITY_EVENTS) {
      window.addEventListener(name, this.recordActivity, { passive: true });
    }
    document.addEventListener("visibilitychange", this.handleVisibility);
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

  // Audio started or stopped playing, with "playback" timing.
  setPlaying(playing: boolean) {
    if (!this.started || this.options.timing !== "playback") return;
    if (!playing) {
      this.end("paused", Date.now());
    } else if (!this.session) {
      const now = Date.now();
      if (!this.sitting || now - this.sitting.pausedAt > READING_IDLE_MS) {
        this.sitting = { startedAt: now, playedMs: 0, pausedAt: now };
      }
      this.session = {
        startedAt: now,
        startPercentage: this.options.getPercentage(),
        lastActivityAt: now,
        page: null,
        pageSince: now,
        countedMs: 0,
      };
    }
  }

  // This sitting of listening so far, or null before anything has played.
  listening(): { since: string; seconds: number } | null {
    if (!this.sitting) return null;
    if (Date.now() - this.sitting.startedAt > MAX_SITTING_MS) return null;
    const running = this.session ? Date.now() - this.session.startedAt : 0;
    return {
      since: new Date(this.sitting.startedAt).toISOString(),
      seconds: (this.sitting.playedMs + running) / 1000,
    };
  }

  recordActivity = () => {
    if (
      !this.started ||
      this.options.timing === "playback" ||
      document.visibilityState !== "visible"
    ) {
      return;
    }
    const now = Date.now();
    const page = this.options.getPage();
    if (!this.session) {
      this.session = {
        startedAt: now,
        startPercentage: null,
        lastActivityAt: now,
        page,
        pageSince: now,
        countedMs: 0,
      };
    }
    const session = this.session;
    if (page !== session.page) {
      // Before the first page shows, the File was loading.
      if (session.page === null) session.startedAt = now;
      else session.countedMs += pageTime(now - session.pageSince);
      session.page = page;
      session.pageSince = now;
    }
    session.startPercentage ??= this.options.getPercentage();
    session.lastActivityAt = now;
    if (this.idleTimer) clearTimeout(this.idleTimer);
    this.idleTimer = setTimeout(() => {
      this.end(
        "idle",
        session.page === null
          ? session.lastActivityAt
          : Math.max(session.lastActivityAt, session.pageSince + MAX_PAGE_MS),
      );
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
    if (session && this.sitting) {
      this.sitting.playedMs += endedAt - session.startedAt;
      this.sitting.pausedAt = endedAt;
    }
    if (!session || endedAt <= session.startedAt) return;
    // Playback counts all of its time.
    let duration: number | undefined;
    if (this.options.timing !== "playback") {
      const countedMs =
        session.page === null
          ? session.countedMs
          : session.countedMs + pageTime(endedAt - session.pageSince);
      duration = Math.round(countedMs / 1000);
      if (duration === 0) return;
    }
    const endPercentage =
      this.options.getPercentage() ?? session.startPercentage ?? 0;
    this.options.onSessionEnd(
      {
        started_at: new Date(session.startedAt).toISOString(),
        ended_at: new Date(endedAt).toISOString(),
        start_percentage: session.startPercentage ?? endPercentage,
        end_percentage: endPercentage,
        duration,
      },
      reason,
    );
  }
}
