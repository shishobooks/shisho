import { Loader2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  useAudnexusChapters,
  type AudnexusChaptersResponse,
} from "@/hooks/queries/audnexus";
import type { ChapterInput } from "@/types";

import {
  applyTitlesAndTimestamps,
  applyTitlesOnly,
  detectIntroOffset,
} from "./audnexusChapterUtils";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const ASIN_RE = /^[A-Z0-9]{10}$/;

function normalizeAsin(raw: string): string {
  return raw.trim().toUpperCase();
}

function isValidAsin(asin: string): boolean {
  return ASIN_RE.test(asin);
}

/** Format milliseconds as "Xh Ym Zs" / "Ym Zs" / "Zs". */
function formatDurationMs(ms: number): string {
  const totalSeconds = Math.round(ms / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  if (hours > 0) {
    return `${hours}h ${minutes}m ${seconds}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds}s`;
  }
  return `${seconds}s`;
}

function errorMessage(code: string | undefined): string {
  switch (code) {
    case "not_found":
      return "We couldn't find this ASIN on Audible. Double-check the ID on the Audible book page.";
    case "timeout":
      return "Request timed out. Try again.";
    case "invalid_asin":
      return "Check the ASIN format. It should be 10 alphanumeric characters.";
    case "rate_limited":
      return "Audible is rate-limiting lookups right now. Wait a few minutes and try again.";
    default:
      return "Couldn't reach Audible. Try again in a moment.";
  }
}

// ---------------------------------------------------------------------------
// Sub-stage components
// ---------------------------------------------------------------------------

interface EntryStageProps {
  asinInput: string;
  onAsinChange: (value: string) => void;
  onFetch: () => void;
}

const EntryStage = ({ asinInput, onAsinChange, onFetch }: EntryStageProps) => {
  const normalized = normalizeAsin(asinInput);
  const valid = isValidAsin(normalized);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" && valid) {
      onFetch();
    }
  };

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="asin-input">Audible ASIN</Label>
        <Input
          className="font-mono"
          id="asin-input"
          maxLength={10}
          onChange={(e) => onAsinChange(e.target.value.toUpperCase())}
          onKeyDown={handleKeyDown}
          placeholder="B0XXXXXXXX"
          value={asinInput}
        />
        <p className="text-xs text-muted-foreground">
          10-character code from the Audible book page URL.
        </p>
      </div>
      <Button disabled={!valid} onClick={onFetch} type="button">
        Fetch chapters
      </Button>
    </div>
  );
};

const LoadingStage = () => (
  <div className="flex flex-col items-center gap-3 py-8 text-muted-foreground">
    <Loader2 className="h-6 w-6 animate-spin" />
    <span className="text-sm">Looking up chapters on Audible...</span>
  </div>
);

interface ErrorStageProps {
  code: string | undefined;
  onRetry: () => void;
}

const ErrorStage = ({ code, onRetry }: ErrorStageProps) => (
  <div className="space-y-4">
    <div className="rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3">
      <p className="text-sm font-medium text-destructive">Lookup failed</p>
      <p className="mt-1 text-sm text-destructive/80">{errorMessage(code)}</p>
    </div>
    <Button onClick={onRetry} type="button" variant="outline">
      Retry
    </Button>
  </div>
);

interface ResultStageProps {
  data: AudnexusChaptersResponse;
  editedChapters: ChapterInput[];
  fileDurationMs: number;
  hasChanges: boolean;
  onApply: (chapters: ChapterInput[]) => void;
  onClose: () => void;
}

const ResultStage = ({
  data,
  editedChapters,
  fileDurationMs,
  hasChanges,
  onApply,
  onClose,
}: ResultStageProps) => {
  const detection = useMemo(
    () =>
      detectIntroOffset({
        runtimeMs: data.runtime_length_ms,
        introMs: data.brand_intro_duration_ms,
        outroMs: data.brand_outro_duration_ms,
        fileDurationMs,
      }),
    [data, fileDurationMs],
  );

  const [applyOffset, setApplyOffset] = useState(detection.applyOffset);

  // Re-seed when new data arrives.
  useEffect(() => {
    setApplyOffset(detection.applyOffset);
  }, [detection.applyOffset]);

  const countsMatch = data.chapters.length === editedChapters.length;
  const diffMs = Math.abs(data.runtime_length_ms - fileDurationMs);

  const handleTitlesOnly = () => {
    const result = applyTitlesOnly(editedChapters, data.chapters);
    onApply(result);
    onClose();
  };

  const handleTitlesAndTimestamps = () => {
    const result = applyTitlesAndTimestamps(data.chapters, {
      applyIntroOffset: applyOffset,
      introMs: data.brand_intro_duration_ms,
    });
    onApply(result);
    onClose();
  };

  const titlesOnlyDisabledTitle = countsMatch
    ? undefined
    : `Chapter counts don't match (${editedChapters.length} vs ${data.chapters.length}).`;

  return (
    <div className="space-y-4">
      {/* Duration comparison */}
      <div className="text-sm text-muted-foreground">
        <span>
          Audible runtime:{" "}
          <span className="font-mono">
            {formatDurationMs(data.runtime_length_ms)}
          </span>
        </span>
        <span className="mx-2 text-muted-foreground/50">·</span>
        <span>
          File duration:{" "}
          <span className="font-mono">{formatDurationMs(fileDurationMs)}</span>
        </span>
      </div>

      {/* Duration match / mismatch callout */}
      {!detection.withinTolerance ? (
        <div className="rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3">
          <p className="text-sm font-medium text-destructive">
            Duration differs by {formatDurationMs(diffMs)}. May be a different
            edition.
          </p>
          <p className="mt-1 text-sm text-destructive/80">
            Chapter timestamps may not align with this file. Trim detection
            requires durations to match within 2 seconds.
          </p>
        </div>
      ) : detection.applyOffset ? (
        <div className="rounded-md border border-primary/30 bg-primary/5 px-4 py-3">
          <p className="text-sm font-medium">
            Intro removed. Chapters offset by -
            {formatDurationMs(data.brand_intro_duration_ms)}.
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            Matches a trimmed file (e.g. Libation rip). Chapters will be shifted
            to align.
          </p>
        </div>
      ) : (
        <div className="rounded-md border border-primary/30 bg-primary/5 px-4 py-3">
          <p className="text-sm font-medium">
            Durations match. File is intact.
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            Chapter timestamps will be used as-is.
          </p>
        </div>
      )}

      {/* Intro offset checkbox — only meaningful when Audible reports a non-zero intro */}
      {data.brand_intro_duration_ms > 0 && (
        <div className="flex items-center gap-2">
          <Checkbox
            checked={applyOffset}
            id="apply-offset"
            onCheckedChange={(checked) => setApplyOffset(Boolean(checked))}
          />
          <Label className="cursor-pointer" htmlFor="apply-offset">
            Shift timestamps: subtract intro (
            {formatDurationMs(data.brand_intro_duration_ms)}) from each chapter
          </Label>
        </div>
      )}

      {/* Chapter count info */}
      <p className="text-sm text-muted-foreground">
        {data.chapters.length} chapter{data.chapters.length !== 1 ? "s" : ""}{" "}
        from Audible
        {!countsMatch && (
          <span className="ml-1 text-destructive">
            (file has {editedChapters.length})
          </span>
        )}
      </p>

      {/* is_accurate flag from Audnexus — warn when timestamps are approximate */}
      {!data.is_accurate && (
        <p className="text-sm text-muted-foreground">
          Note: Audible flagged this entry as approximate. Spot-check timestamps
          before saving.
        </p>
      )}

      {/* Overwrite warning */}
      {hasChanges && (
        <div className="rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3">
          <p className="text-sm font-medium text-destructive">
            Unsaved changes will be overwritten
          </p>
          <p className="mt-1 text-sm text-destructive/80">
            Applying will replace your current edits. Canceling keeps this
            dialog open but your unsaved edits are still pending.
          </p>
        </div>
      )}

      {/* Action buttons */}
      <div className="flex flex-col sm:flex-row gap-2">
        {countsMatch ? (
          <>
            <Button onClick={handleTitlesOnly} type="button">
              Apply titles only
            </Button>
            <Button
              onClick={handleTitlesAndTimestamps}
              type="button"
              variant="outline"
            >
              Apply titles + timestamps
            </Button>
          </>
        ) : (
          <>
            <Button onClick={handleTitlesAndTimestamps} type="button">
              Apply titles + timestamps
            </Button>
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="inline-flex">
                  <Button
                    className="w-full"
                    disabled
                    type="button"
                    variant="outline"
                  >
                    Apply titles only
                  </Button>
                </span>
              </TooltipTrigger>
              <TooltipContent side="bottom">
                {titlesOnlyDisabledTitle}
              </TooltipContent>
            </Tooltip>
          </>
        )}
        <Button
          className="sm:ml-auto"
          onClick={onClose}
          type="button"
          variant="ghost"
        >
          Cancel
        </Button>
      </div>
    </div>
  );
};

// ---------------------------------------------------------------------------
// Public props + component
// ---------------------------------------------------------------------------

export interface FetchChaptersDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** ASIN prefilled from the file's existing identifiers, if present. */
  initialAsin?: string;
  /** Called when the user clicks an Apply button. Closes the dialog. */
  onApply: (chapters: ChapterInput[]) => void;
  /** Current edited chapters; used for the counts-match check. */
  editedChapters: ChapterInput[];
  /** File duration in ms; used for trim offset detection. */
  fileDurationMs: number;
  /** True if the parent edit form has unsaved edits. */
  hasChanges: boolean;
}

type Stage = "entry" | "loading" | "result" | "error";

const FetchChaptersDialog = ({
  open,
  onOpenChange,
  initialAsin,
  onApply,
  editedChapters,
  fileDurationMs,
  hasChanges,
}: FetchChaptersDialogProps) => {
  const [asinInput, setAsinInput] = useState(
    initialAsin ? normalizeAsin(initialAsin) : "",
  );
  const [submittedAsin, setSubmittedAsin] = useState<string | null>(null);

  // Reset to entry stage and reseed ASIN whenever the dialog opens.
  useEffect(() => {
    if (open) {
      setAsinInput(initialAsin ? normalizeAsin(initialAsin) : "");
      setSubmittedAsin(null);
    }
  }, [open, initialAsin]);

  const query = useAudnexusChapters(submittedAsin, {
    enabled: Boolean(submittedAsin),
  });

  const stage: Stage = useMemo((): Stage => {
    if (!submittedAsin) return "entry";
    if (query.isSuccess) return "result";
    if (query.isError) return "error";
    return "loading";
  }, [submittedAsin, query.isSuccess, query.isError]);

  const handleFetch = () => {
    const normalized = normalizeAsin(asinInput);
    if (!isValidAsin(normalized)) return;
    setSubmittedAsin(normalized);
  };

  const handleRetry = () => {
    // Force a fresh network call rather than replaying a cached error from
    // tanstack-query.
    void query.refetch();
  };

  const handleClose = () => {
    onOpenChange(false);
  };

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-lg overflow-x-hidden">
        <DialogHeader className="pr-8">
          <DialogTitle>Fetch chapters from Audible</DialogTitle>
          <DialogDescription>
            Look up chapter titles and timestamps using an Audible ASIN.
          </DialogDescription>
        </DialogHeader>

        {stage === "entry" && (
          <EntryStage
            asinInput={asinInput}
            onAsinChange={setAsinInput}
            onFetch={handleFetch}
          />
        )}

        {stage === "loading" && <LoadingStage />}

        {stage === "error" && (
          <ErrorStage code={query.error?.code} onRetry={handleRetry} />
        )}

        {stage === "result" && query.data && (
          <ResultStage
            data={query.data}
            editedChapters={editedChapters}
            fileDurationMs={fileDurationMs}
            hasChanges={hasChanges}
            onApply={onApply}
            onClose={handleClose}
          />
        )}
      </DialogContent>
    </Dialog>
  );
};

export default FetchChaptersDialog;
