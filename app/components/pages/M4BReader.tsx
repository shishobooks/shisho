import {
  ArrowLeft,
  Pause,
  Play,
  RotateCcw,
  RotateCw,
  SkipBack,
  SkipForward,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";

import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Slider } from "@/components/ui/slider";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { usePageTitle } from "@/hooks/usePageTitle";
import { cn } from "@/libraries/utils";
import {
  PlaybackSpeeds,
  type Book,
  type File,
  type PlaybackSpeed,
} from "@/types";
import {
  resolveChapters,
  resolvePlayback,
  resolveSkipTarget,
  SKIP_SECONDS,
} from "@/utils/chapters";
import { formatPlayerTime } from "@/utils/format";

interface M4BReaderProps {
  file: File;
  book?: Book;
  libraryId: string;
}

function joinNames(
  people: Array<{ person?: { name?: string } }> | undefined,
): string {
  if (!people) return "";
  return people
    .map((p) => p.person?.name)
    .filter((name): name is string => Boolean(name))
    .join(", ");
}

export default function M4BReader({ file, book, libraryId }: M4BReaderProps) {
  const navigate = useNavigate();
  const audioRef = useRef<HTMLAudioElement>(null);

  usePageTitle(book?.title ? `Listening: ${book.title}` : "Audiobook Player");

  const streamUrl = `/api/books/files/${file.id}/stream`;

  // Authoritative total: prefer the duration stored on the model (available
  // immediately, before the audio element loads its metadata), fall back to
  // the element's own reported duration once metadata arrives.
  const [mediaDuration, setMediaDuration] = useState(0);
  const duration = useMemo(() => {
    const fromFile = file.audiobook_duration_seconds;
    if (fromFile && fromFile > 0) return fromFile;
    return mediaDuration;
  }, [file.audiobook_duration_seconds, mediaDuration]);

  const [currentTime, setCurrentTime] = useState(0);
  const [isPlaying, setIsPlaying] = useState(false);
  // Keep a ref in sync so the stable togglePlay callback can read the latest
  // playing state without resubscribing the document keydown listener.
  const isPlayingRef = useRef(false);
  useEffect(() => {
    isPlayingRef.current = isPlaying;
  }, [isPlaying]);

  // Keep the live time and duration in refs so the stable keydown handler can
  // compute skip targets from the latest values without resubscribing on every
  // timeupdate — and, crucially, without lagging a render behind, so successive
  // synchronous arrow presses each build on the prior one's seek.
  const currentTimeRef = useRef(0);
  const durationRef = useRef(duration);
  useEffect(() => {
    durationRef.current = duration;
  }, [duration]);

  // While the user is dragging the seek bar we follow the pending scrub value
  // instead of the audio's reported time, so timeupdate events don't fight the
  // thumb. On commit we seek the element and resume following timeupdate.
  const [isScrubbing, setIsScrubbing] = useState(false);
  const [scrubValue, setScrubValue] = useState(0);

  const [coverError, setCoverError] = useState(false);

  // Playback speed is a per-user setting so the chosen rate follows the
  // listener across sessions and devices. Local state applies the rate
  // immediately on change; the mutation persists it through the standard
  // user-settings endpoint, and the effect below re-syncs if the server
  // value changes from elsewhere.
  const { data: settings } = useUserSettings();
  const updateSettings = useUpdateUserSettings();
  const persistedSpeed = settings?.viewer_playback_speed ?? 1;
  const [playbackSpeed, setPlaybackSpeed] =
    useState<PlaybackSpeed>(persistedSpeed);
  useEffect(() => {
    setPlaybackSpeed(persistedSpeed);
  }, [persistedSpeed]);

  // Apply the rate to the audio element whenever it changes (including the
  // initial render, restoring the persisted speed).
  useEffect(() => {
    const audio = audioRef.current;
    if (audio) {
      audio.playbackRate = playbackSpeed;
    }
  }, [playbackSpeed]);

  const handleSpeedChange = useCallback(
    (value: string) => {
      // Values come from the PlaybackSpeeds list, so the cast is safe.
      const speed = Number(value) as PlaybackSpeed;
      setPlaybackSpeed(speed);
      updateSettings.mutate({ viewer_playback_speed: speed });
    },
    [updateSettings],
  );

  // Resolve the file's chapters into absolute second boundaries once. The pure
  // module handles unit conversion (ms -> s), sorting, and dropping chapters
  // without a start. A file with no usable chapters yields an empty list and
  // chapter navigation is simply absent.
  const chapters = useMemo(
    () => resolveChapters(file.chapters, duration),
    [file.chapters, duration],
  );
  const hasChapters = chapters.length > 0;

  // The displayed/effective time follows the scrub value while dragging so the
  // chapter readout and dropdown track the thumb, otherwise the audio's time.
  const displayTime = isScrubbing ? scrubValue : currentTime;

  // Derive all playback-relative targets (current chapter, prev/next, skips)
  // from the pure module. Recomputed as time/chapters/duration change.
  const playback = useMemo(
    () => resolvePlayback(chapters, displayTime, duration),
    [chapters, displayTime, duration],
  );

  // Seek the audio element and keep our currentTime in sync. Centralized so the
  // buttons, dropdown, and keyboard shortcuts all go through one path. Writes
  // the time ref eagerly (before the state update flushes) so the keydown
  // handler always reads the freshest position.
  const seekTo = useCallback((seconds: number) => {
    const audio = audioRef.current;
    if (audio) {
      audio.currentTime = seconds;
    }
    currentTimeRef.current = seconds;
    setCurrentTime(seconds);
  }, []);

  const togglePlay = useCallback(() => {
    const audio = audioRef.current;
    if (!audio) return;
    // Drive the decision off our own playing state (kept in sync by the
    // play/pause events) rather than audio.paused, which is more robust and
    // testable. play() returns a promise that can reject (e.g. if interrupted);
    // the play/pause events remain the source of truth, so swallow.
    if (isPlayingRef.current) {
      audio.pause();
    } else {
      void audio.play().catch(() => {});
    }
  }, []);

  // Space toggles play/pause; Left/Right arrows skip back/forward by 30s. Handle
  // at the document level and preventDefault so the page doesn't scroll. Ignore
  // the keys when focus is in a text input or when modifier keys are held so we
  // don't hijack browser shortcuts. The chapter dropdown (a Radix Select), while
  // open, handles its own arrow-key navigation and stops propagation, so its
  // keys never reach this document-level listener.
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey || e.altKey) return;
      const target = e.target as HTMLElement | null;
      const tag = target?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || target?.isContentEditable) {
        return;
      }

      if (e.key === " " || e.key === "Spacebar") {
        e.preventDefault();
        togglePlay();
        return;
      }
      if (e.key === "ArrowLeft") {
        e.preventDefault();
        seekTo(
          resolveSkipTarget(
            currentTimeRef.current,
            -SKIP_SECONDS,
            durationRef.current,
          ),
        );
        return;
      }
      if (e.key === "ArrowRight") {
        e.preventDefault();
        seekTo(
          resolveSkipTarget(
            currentTimeRef.current,
            SKIP_SECONDS,
            durationRef.current,
          ),
        );
        return;
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [togglePlay, seekTo]);

  const handleSeekChange = useCallback(([value]: number[]) => {
    setIsScrubbing(true);
    setScrubValue(value);
  }, []);

  const handleSeekCommit = useCallback(
    ([value]: number[]) => {
      seekTo(value);
      setIsScrubbing(false);
    },
    [seekTo],
  );

  const authorNames = joinNames(book?.authors);
  const narratorNames = joinNames(file.narrators);

  const coverCacheKey = book?.cover_cache_key;
  const coverUrl =
    book?.id != null
      ? coverCacheKey
        ? `/api/books/${book.id}/cover?v=${coverCacheKey}`
        : `/api/books/${book.id}/cover`
      : null;

  const sliderMax = duration > 0 ? duration : 1;

  return (
    <div className="fixed inset-0 bg-background flex flex-col">
      {/* Header with a way out of the player */}
      <header className="flex items-center gap-2 px-4 py-2 border-b">
        <Button
          aria-label="Back"
          onClick={() => {
            if (book?.id != null) {
              navigate(`/libraries/${libraryId}/books/${book.id}`);
            } else {
              navigate(-1);
            }
          }}
          size="icon"
          variant="ghost"
        >
          <ArrowLeft className="h-5 w-5" />
        </Button>
        <span className="truncate text-sm font-medium">
          {book?.title ?? "Audiobook"}
        </span>
      </header>

      {/* Main content: cover + metadata, stacks on small screens */}
      <main className="flex-1 overflow-y-auto">
        <div className="mx-auto flex h-full max-w-2xl flex-col items-center justify-center gap-6 p-4 md:p-8">
          <div className="aspect-square w-48 sm:w-56 md:w-64 shrink-0">
            {coverUrl && !coverError ? (
              <img
                alt={`${book?.title ?? "Audiobook"} cover`}
                className="h-full w-full rounded-md border border-border object-cover shadow-sm"
                key={coverCacheKey}
                onError={() => setCoverError(true)}
                src={coverUrl}
              />
            ) : (
              <CoverPlaceholder
                className="h-full w-full rounded-md border border-border"
                variant="audiobook"
              />
            )}
          </div>

          <div className="w-full text-center">
            <h1 className="text-xl font-semibold break-words md:text-2xl">
              {book?.title ?? "Audiobook"}
            </h1>
            {authorNames && (
              <p className="mt-1 text-sm text-muted-foreground break-words">
                {authorNames}
              </p>
            )}
            {narratorNames && (
              <p className="mt-1 text-sm text-muted-foreground break-words">
                Narrated by {narratorNames}
              </p>
            )}
          </div>
        </div>
      </main>

      {/* Player controls — always visible, no auto-hide */}
      <footer className="border-t bg-background px-4 py-4 md:px-8">
        <div className="mx-auto max-w-2xl space-y-3">
          {/* Chapter dropdown (only with chapters). Its trigger doubles as the
              current chapter title, kept in sync with playback via `value`. */}
          {hasChapters && (
            <Select
              onValueChange={(value) => {
                const chapter = chapters[Number(value)];
                if (chapter) seekTo(chapter.startSeconds);
              }}
              value={
                playback.currentChapterIndex != null
                  ? String(playback.currentChapterIndex)
                  : undefined
              }
            >
              <SelectTrigger aria-label="Chapter" className="w-full min-w-0">
                <SelectValue placeholder="Select chapter" />
              </SelectTrigger>
              <SelectContent>
                {chapters.map((chapter) => (
                  <SelectItem key={chapter.index} value={String(chapter.index)}>
                    <span className="truncate" title={chapter.title}>
                      {chapter.title}
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}

          <div className="relative">
            <Slider
              max={sliderMax}
              min={0}
              onValueChange={handleSeekChange}
              onValueCommit={handleSeekCommit}
              step={1}
              thumbLabel="Seek"
              value={[Math.min(displayTime, sliderMax)]}
            />
            {/* Chapter markers along the seek bar, positioned by start/duration.
                Skip the first marker at 0 (it sits under the thumb origin). */}
            {hasChapters &&
              duration > 0 &&
              chapters.map((chapter) =>
                chapter.startSeconds <= 0 ? null : (
                  <span
                    aria-hidden="true"
                    className="pointer-events-none absolute top-1/2 h-2 w-px -translate-x-1/2 -translate-y-1/2 bg-primary/40"
                    key={chapter.index}
                    style={{
                      left: `${(chapter.startSeconds / duration) * 100}%`,
                    }}
                  />
                ),
              )}
          </div>

          <div className="flex items-center justify-between text-xs tabular-nums text-muted-foreground">
            <span>{formatPlayerTime(displayTime)}</span>
            {/* Speed control sits between the time labels; it applies the
                rate immediately and persists it as a per-user setting. */}
            <Select
              onValueChange={handleSpeedChange}
              value={String(playbackSpeed)}
            >
              <SelectTrigger
                aria-label="Playback speed"
                className="h-7 w-auto min-w-0 gap-1 px-2 text-xs"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PlaybackSpeeds.map((speed) => (
                  <SelectItem key={speed} value={String(speed)}>
                    {speed}x
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <span>{formatPlayerTime(duration)}</span>
          </div>

          <div className="flex items-center justify-center gap-2 sm:gap-4">
            {hasChapters && (
              <Button
                aria-label="Previous chapter"
                disabled={playback.previousChapterTarget == null}
                onClick={() => {
                  if (playback.previousChapterTarget != null) {
                    seekTo(playback.previousChapterTarget);
                  }
                }}
                size="icon"
                variant="ghost"
              >
                <SkipBack className="h-5 w-5" />
              </Button>
            )}
            <Button
              aria-label="Skip back 30 seconds"
              onClick={() => seekTo(playback.skipBackTarget)}
              size="icon"
              variant="ghost"
            >
              <RotateCcw className="h-5 w-5" />
            </Button>
            <Button
              aria-label={isPlaying ? "Pause" : "Play"}
              className={cn("h-14 w-14 rounded-full")}
              onClick={togglePlay}
              size="icon"
            >
              {isPlaying ? (
                <Pause className="h-6 w-6" />
              ) : (
                <Play className="h-6 w-6" />
              )}
            </Button>
            <Button
              aria-label="Skip forward 30 seconds"
              onClick={() => seekTo(playback.skipForwardTarget)}
              size="icon"
              variant="ghost"
            >
              <RotateCw className="h-5 w-5" />
            </Button>
            {hasChapters && (
              <Button
                aria-label="Next chapter"
                disabled={playback.nextChapterTarget == null}
                onClick={() => {
                  if (playback.nextChapterTarget != null) {
                    seekTo(playback.nextChapterTarget);
                  }
                }}
                size="icon"
                variant="ghost"
              >
                <SkipForward className="h-5 w-5" />
              </Button>
            )}
          </div>
        </div>
      </footer>

      <audio
        onEnded={() => setIsPlaying(false)}
        onLoadedMetadata={(e) => {
          const el = e.currentTarget;
          if (Number.isFinite(el.duration) && el.duration > 0) {
            setMediaDuration(el.duration);
          }
          // The media load algorithm resets playbackRate to the default as
          // part of loading the resource, which can land after the effect
          // applied the persisted speed. Re-apply it once metadata arrives.
          el.playbackRate = playbackSpeed;
        }}
        onPause={() => setIsPlaying(false)}
        onPlay={() => setIsPlaying(true)}
        onTimeUpdate={(e) => {
          if (isScrubbing) return;
          const time = e.currentTarget.currentTime;
          currentTimeRef.current = time;
          setCurrentTime(time);
        }}
        preload="metadata"
        ref={audioRef}
        src={streamUrl}
      />
    </div>
  );
}
