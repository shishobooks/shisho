import { ArrowLeft, Pause, Play } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";

import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { Button } from "@/components/ui/button";
import { Slider } from "@/components/ui/slider";
import { usePageTitle } from "@/hooks/usePageTitle";
import { cn } from "@/libraries/utils";
import type { Book, File } from "@/types";
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

  // While the user is dragging the seek bar we follow the pending scrub value
  // instead of the audio's reported time, so timeupdate events don't fight the
  // thumb. On commit we seek the element and resume following timeupdate.
  const [isScrubbing, setIsScrubbing] = useState(false);
  const [scrubValue, setScrubValue] = useState(0);

  const [coverError, setCoverError] = useState(false);

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

  // Space toggles play/pause. Handle at the document level and preventDefault
  // so the page doesn't scroll. Ignore the key when focus is in a text input
  // (none today, but keeps the behavior robust) and when modifier keys are
  // held so we don't hijack browser shortcuts.
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key !== " " && e.key !== "Spacebar") return;
      if (e.ctrlKey || e.metaKey || e.altKey) return;
      const target = e.target as HTMLElement | null;
      const tag = target?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || target?.isContentEditable) {
        return;
      }
      e.preventDefault();
      togglePlay();
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [togglePlay]);

  const handleSeekChange = useCallback(([value]: number[]) => {
    setIsScrubbing(true);
    setScrubValue(value);
  }, []);

  const handleSeekCommit = useCallback(([value]: number[]) => {
    const audio = audioRef.current;
    if (audio) {
      audio.currentTime = value;
    }
    setCurrentTime(value);
    setIsScrubbing(false);
  }, []);

  const displayTime = isScrubbing ? scrubValue : currentTime;

  const authorNames = joinNames(book?.authors);
  const narratorNames = joinNames(file.narrators);

  const coverCacheKey = book?.cover_cache_key;
  const coverUrl =
    book?.id != null
      ? coverCacheKey
        ? `/api/books/${book.id}/cover?v=${coverCacheKey}`
        : `/api/books/${book.id}/cover`
      : null;

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
          <Slider
            max={duration > 0 ? duration : 1}
            min={0}
            onValueChange={handleSeekChange}
            onValueCommit={handleSeekCommit}
            step={1}
            thumbLabel="Seek"
            value={[Math.min(displayTime, duration > 0 ? duration : 1)]}
          />
          <div className="flex items-center justify-between text-xs tabular-nums text-muted-foreground">
            <span>{formatPlayerTime(displayTime)}</span>
            <span>{formatPlayerTime(duration)}</span>
          </div>
          <div className="flex items-center justify-center">
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
        }}
        onPause={() => setIsPlaying(false)}
        onPlay={() => setIsPlaying(true)}
        onTimeUpdate={(e) => {
          if (isScrubbing) return;
          setCurrentTime(e.currentTarget.currentTime);
        }}
        preload="metadata"
        ref={audioRef}
        src={streamUrl}
      />
    </div>
  );
}
