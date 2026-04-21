import type { AudnexusChapter } from "@/hooks/queries/audnexus";
import type { ChapterInput } from "@/types";

export const TRIM_DETECT_TOLERANCE_MS = 2000;

interface DetectIntroOffsetParams {
  runtimeMs: number;
  introMs: number;
  outroMs: number;
  fileDurationMs: number;
}

interface DetectIntroOffsetResult {
  applyOffset: boolean;
  withinTolerance: boolean;
}

/**
 * Picks whether to apply an intro offset based on which Audnexus duration
 * candidate is closest to the file's actual duration. Candidates are:
 *   1. runtime (intact)
 *   2. runtime - intro
 *   3. runtime - outro
 *   4. runtime - intro - outro
 *
 * If the closest candidate subtracts intro, applyOffset=true. Otherwise false.
 * If no candidate is within TRIM_DETECT_TOLERANCE_MS, withinTolerance=false
 * (UI shows a mismatch warning) and applyOffset defaults to the closest match.
 */
export const detectIntroOffset = (
  params: DetectIntroOffsetParams,
): DetectIntroOffsetResult => {
  const { runtimeMs, introMs, outroMs, fileDurationMs } = params;
  const candidates: { ms: number; subtractsIntro: boolean }[] = [
    { ms: runtimeMs, subtractsIntro: false },
    { ms: runtimeMs - introMs, subtractsIntro: true },
    { ms: runtimeMs - outroMs, subtractsIntro: false },
    { ms: runtimeMs - introMs - outroMs, subtractsIntro: true },
  ];

  let best = candidates[0];
  let bestDiff = Math.abs(candidates[0].ms - fileDurationMs);
  for (let i = 1; i < candidates.length; i++) {
    const diff = Math.abs(candidates[i].ms - fileDurationMs);
    if (diff < bestDiff) {
      best = candidates[i];
      bestDiff = diff;
    }
  }

  const withinTolerance = bestDiff <= TRIM_DETECT_TOLERANCE_MS;
  return {
    applyOffset: withinTolerance ? best.subtractsIntro : false,
    withinTolerance,
  };
};

/**
 * Replace titles in-place by position; leave timestamps and children alone.
 * Returns the original array unchanged when lengths differ so the caller
 * doesn't silently lose data.
 */
export const applyTitlesOnly = (
  existing: ChapterInput[],
  fromAudible: AudnexusChapter[],
): ChapterInput[] => {
  if (existing.length !== fromAudible.length) {
    return existing;
  }
  return existing.map((chapter, i) => ({
    ...chapter,
    title: fromAudible[i].title,
  }));
};

interface ApplyTitlesAndTimestampsOpts {
  applyIntroOffset: boolean;
  introMs: number;
}

/**
 * Build a fresh chapter array from Audnexus data, optionally subtracting the
 * intro duration from every start timestamp. Negative results are clamped
 * to 0 (only possible for the first chapter).
 */
export const applyTitlesAndTimestamps = (
  fromAudible: AudnexusChapter[],
  opts: ApplyTitlesAndTimestampsOpts,
): ChapterInput[] => {
  const offset = opts.applyIntroOffset ? opts.introMs : 0;
  return fromAudible.map((c) => ({
    title: c.title,
    start_timestamp_ms: Math.max(0, c.start_offset_ms - offset),
    children: [],
  }));
};
