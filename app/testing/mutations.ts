import { vi } from "vitest";

import { ShishoAPIError } from "@/libraries/api";

/** The message a mutation rejected by `rejectingMutate` carries. */
export const REJECTION_MESSAGE = "The server could not save this.";

type MutateOptions = { onError?: (error: unknown) => void };

/**
 * A stand-in for a mutation's `mutate` that always fails: it calls the
 * caller's `onError` with a `ShishoAPIError`, the way TanStack Query reports
 * a rejected request. A caller that passes no `onError` drops the failure,
 * which is what tests using this catch.
 */
export const rejectingMutate = () =>
  vi.fn((_variables: unknown, options?: MutateOptions) => {
    options?.onError?.(new ShishoAPIError(REJECTION_MESSAGE, "internal", 500));
  });

/**
 * Records the promise rejections nothing handled while it watches. A click
 * handler that awaits a rejected mutation without a catch leaves one behind.
 * `stop()` waits a macrotask so Node has reported any such rejection, then
 * stops watching and returns what it saw.
 *
 * Two limits: reject from a plain async function, not a `vi.fn()` mock,
 * because Vitest's spy handles the promise its mock returns and so hides the
 * leak; and do not call `stop()` under `vi.useFakeTimers()` without
 * `shouldAdvanceTime`, since it waits on a real `setTimeout`.
 */
export const watchUnhandledRejections = () => {
  const reasons: unknown[] = [];
  const listener = (reason: unknown) => {
    reasons.push(reason);
  };
  process.on("unhandledRejection", listener);
  return {
    stop: async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
      process.off("unhandledRejection", listener);
      return reasons;
    },
  };
};
