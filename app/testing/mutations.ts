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
