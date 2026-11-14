import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useRef } from "react";

import {
  dismissReadingFinishedToast,
  showReadingFinishedToast,
} from "@/components/library/ReadingFinishedToast";
import {
  invalidateAfterPositionUpdate,
  markProgressStale,
  QueryKey,
  saveFilePosition,
  useUndoAutoFinish,
} from "@/hooks/queries/reading";
import { useAuth } from "@/hooks/useAuth";
import { toastRequestError } from "@/libraries/api";
import {
  ReadingSessionTracker,
  type SessionTiming,
} from "@/libraries/readingSession";
import {
  ReadingSourceWebReader,
  type FilePositionResponse,
  type SessionPayload,
} from "@/types";

// Saves wait this long after a move, so turning several pages sends one.
export const POSITION_SAVE_DELAY_MS = 3000;

// Saves the reader wants sent at once stay this far apart, so a held key sends
// a few saves and not one for every repeat.
const IMMEDIATE_SAVE_SPACING_MS = 500;

// Where a reader is, in the one form it keeps: a location in the text, a
// 0-based page, or seconds.
export interface ReaderPosition {
  cfi?: string;
  page?: number;
  seconds?: number;
  // How far through the File, from 0 to 1.
  percentage: number;
  // The reader is at the end of the File, which finishes the Read-through.
  atEnd: boolean;
  // Which screen the reader shows, when its location changes within one,
  // as a CFI does with every scroll. Reading time counts per screen, and
  // without this, per location.
  screen?: string;
}

interface Tracked extends ReaderPosition {
  // When the reader got there, so a save that arrives late can't replace a
  // newer one from another browser.
  occurredAt: string;
  // The reader opened here from a link, which saves the place but isn't
  // reading.
  linked?: boolean;
}

const sameLocation = (a: ReaderPosition, b: ReaderPosition) =>
  a.cfi === b.cfi && a.page === b.page && a.seconds === b.seconds;

// The page a session's time counts on. Each kind has its own prefix, so a
// page number never matches a screen number.
const pageOf = (position: ReaderPosition) => {
  if (position.screen != null) return `s:${position.screen}`;
  if (position.cfi != null) return `c:${position.cfi}`;
  return position.page != null ? `p:${position.page}` : null;
};

// Keeps a File's Reading Position and Reading Sessions current for a web
// reader. The reader tells it where it opened (opened) and where the reader
// moves (moved); it saves moves a few seconds apart, or as far apart as the
// reader asks, at once at the end and when the page is hidden or closes, and
// times sessions while the reader is mounted. A place a link opened it at goes
// through openedFromLink. Audio passes "playback" timing
// and reports setPlaying. It shows the finish toast when a save finishes the
// Book. In Demo Mode, or when enabled is false, it saves nothing.
export const useReadingProgress = (
  fileId: number,
  {
    enabled = true,
    timing,
    getScreen,
  }: {
    enabled?: boolean;
    timing?: SessionTiming;
    // The screen the reader shows right now, or null where screens don't
    // apply. Reading time counts per screen when this gives one, for a
    // reader that only reports moves between pages longer than a screen.
    getScreen?: () => number | null;
  } = {},
) => {
  const { demoMode } = useAuth();
  const active = enabled && !demoMode;
  const queryClient = useQueryClient();
  const undoMutation = useUndoAutoFinish();
  // The toast's Undo runs after this render's mutation object is stale.
  const undoRef = useRef(undoMutation);
  const getScreenRef = useRef(getScreen);
  useEffect(() => {
    undoRef.current = undoMutation;
    getScreenRef.current = getScreen;
  });

  const currentRef = useRef<Tracked | null>(null);
  const savedRef = useRef<Tracked | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const trackerRef = useRef<ReadingSessionTracker | null>(null);
  // The reader has closed, so nothing is showing the Book's reading record
  // but the page the reader is going back to.
  const closedRef = useRef(false);
  const lastImmediateSaveRef = useRef(0);
  const playingRef = useRef(false);

  const save = useCallback(
    (session?: SessionPayload, keepalive = false) => {
      if (timerRef.current) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
      const current = currentRef.current;
      if (!current) return;
      // Nothing moved since the last save (or since opening, which isn't
      // reading): only a session has anything to report, and it goes alone
      // so where the reader opened can't start or finish a Read-through.
      const previous = savedRef.current;
      const moved = current !== previous;
      if (!moved && !session) return;
      savedRef.current = current;

      // A reader reopened before the save lands opens here, not where the
      // server last said.
      if (moved) {
        queryClient.setQueryData<FilePositionResponse>(
          [QueryKey.FilePosition, fileId],
          (old) => ({
            position: {
              created_at: current.occurredAt,
              updated_at: current.occurredAt,
              ...old?.position,
              file_id: fileId,
              cfi: current.cfi,
              page: current.page,
              seconds: current.seconds,
              percentage: current.percentage,
              source: ReadingSourceWebReader,
              occurred_at: current.occurredAt,
            },
          }),
        );
      }

      const listening = trackerRef.current?.listening();
      saveFilePosition(
        fileId,
        moved
          ? {
              cfi: current.cfi,
              page: current.page,
              seconds: current.seconds,
              percentage: current.percentage,
              at_end: current.atEnd || undefined,
              opened: current.linked || undefined,
              listened_seconds: listening?.seconds,
              listening_since: listening?.since,
              occurred_at: current.occurredAt,
              session,
            }
          : { session },
        keepalive,
      )
        .then((result) => {
          if (
            result.started ||
            result.resumed ||
            result.reverted ||
            result.finished_read_through_id
          ) {
            invalidateAfterPositionUpdate(queryClient, result.book_id, fileId);
          } else if (result.accepted) {
            markProgressStale(queryClient, result.book_id, closedRef.current);
          }
          if (result.reverted) dismissReadingFinishedToast(result.book_id);
          const finishedId = result.finished_read_through_id;
          if (finishedId != null) {
            showReadingFinishedToast({
              bookId: result.book_id,
              onUndo: () =>
                undoRef.current.mutate(finishedId, {
                  onError: (error) =>
                    toastRequestError(error, "Failed to undo the finish"),
                }),
            });
          }
        })
        .catch((error: unknown) => {
          // The next save sends the place again, with when the reader got
          // there, so a dropped save can't lose a move or a finish.
          if (savedRef.current === current) savedRef.current = previous;
          toastRequestError(error, "Failed to save your reading position", {
            id: "reading-position",
          });
        });
    },
    [fileId, queryClient],
  );

  useEffect(() => {
    if (!active) return;
    closedRef.current = false;
    const tracker = new ReadingSessionTracker({
      timing,
      getPercentage: () => currentRef.current?.percentage ?? null,
      getPage: () => {
        const current = currentRef.current;
        if (!current) return null;
        const screen = getScreenRef.current?.();
        return screen != null ? `s:${screen}` : pageOf(current);
      },
      onSessionEnd: (session, reason) => save(session, reason !== "idle"),
    });
    trackerRef.current = tracker;
    // Registered after the tracker's, so a session ending on hide or close
    // goes with the save, and this only catches a move without one.
    const flushIfHidden = () => {
      if (document.visibilityState === "hidden") save(undefined, true);
    };
    const flush = () => save(undefined, true);
    tracker.start();
    tracker.setPlaying(playingRef.current);
    document.addEventListener("visibilitychange", flushIfHidden);
    window.addEventListener("pagehide", flush);
    return () => {
      closedRef.current = true;
      tracker.stop();
      trackerRef.current = null;
      document.removeEventListener("visibilitychange", flushIfHidden);
      window.removeEventListener("pagehide", flush);
      flush();
    };
  }, [active, save, timing]);

  // Where the reader opened, which isn't reading: nothing saves it, and until
  // the reader moves a session goes out with no position.
  const opened = useCallback((position: ReaderPosition) => {
    const tracked = {
      ...position,
      occurredAt: new Date().toISOString(),
    };
    currentRef.current = tracked;
    savedRef.current = tracked;
    // Time on the first page counts from when it shows, not from while the
    // File was loading.
    trackerRef.current?.recordActivity();
  }, []);

  // The reader moved, by the user's doing or by playing on. saveAfterMs is how
  // long the save may wait for more moves; 0 saves now.
  const moved = useCallback(
    (
      position: ReaderPosition,
      { saveAfterMs = POSITION_SAVE_DELAY_MS }: { saveAfterMs?: number } = {},
    ) => {
      if (!active) return;
      const last = currentRef.current;
      if (
        last &&
        sameLocation(last, position) &&
        last.atEnd === position.atEnd
      ) {
        return;
      }
      currentRef.current = {
        ...position,
        occurredAt: new Date().toISOString(),
      };
      trackerRef.current?.recordActivity();
      if (position.atEnd || saveAfterMs === 0) {
        // Sent at once, it has to outlive the page, since closing right after
        // finds nothing left to flush.
        const sendNow = () => {
          lastImmediateSaveRef.current = Date.now();
          save(undefined, true);
        };
        const wait = position.atEnd
          ? 0
          : lastImmediateSaveRef.current +
            IMMEDIATE_SAVE_SPACING_MS -
            Date.now();
        if (wait <= 0) {
          sendNow();
        } else {
          if (timerRef.current) clearTimeout(timerRef.current);
          timerRef.current = setTimeout(sendNow, wait);
        }
      } else timerRef.current ??= setTimeout(() => save(), saveAfterMs);
    },
    [active, save],
  );

  // Where a link opened the reader, when that isn't the saved place. It becomes
  // the saved place, but starts, finishes, and undoes nothing.
  const openedFromLink = useCallback(
    (position: ReaderPosition) => {
      if (!active) return;
      currentRef.current = {
        ...position,
        atEnd: false,
        linked: true,
        occurredAt: new Date().toISOString(),
      };
      // Time on this page counts from when it shows, as in opened.
      trackerRef.current?.recordActivity();
      save(undefined, true);
    },
    [active, save],
  );

  // Audio started or stopped playing. Stopping ends the session, which goes
  // out with any move not yet saved.
  const setPlaying = useCallback(
    (playing: boolean) => {
      playingRef.current = playing;
      trackerRef.current?.setPlaying(playing);
      if (!playing && active) save(undefined, true);
    },
    [active, save],
  );

  // Input the reader saw where the window doesn't, such as inside the
  // book's frame.
  const recordActivity = useCallback(() => {
    trackerRef.current?.recordActivity();
  }, []);

  return { opened, openedFromLink, moved, recordActivity, setPlaying };
};
