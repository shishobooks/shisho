import { AlertTriangle, Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { requestErrorMessage } from "@/libraries/api";
import { cn } from "@/libraries/utils";

/** The parts of a TanStack Query result QueryError reads. Pass the query. */
export interface RetryableQuery {
  error: unknown;
  isFetching: boolean;
  isEnabled: boolean;
  refetch: () => unknown;
}

interface QueryErrorProps {
  query: RetryableQuery;
  /**
   * Names what failed to load ("Failed to load plugins"). Shown unless the
   * server sent a message of its own (see requestErrorMessage).
   */
  fallback: string;
  /** Overrides the box's spacing, e.g. a margin where it replaces a list. */
  className?: string;
}

/**
 * The inline report of a failed query: the message requestErrorMessage
 * derives and, for an enabled query, a Retry button. Every page, section
 * and dialog that shows a query failure renders this, so a failure reads and
 * retries the same way everywhere (see "Request errors" in app/AGENTS.md).
 * A disabled query gets no Retry, since `refetch()` would bypass the hook's
 * permission gate.
 */
const QueryError = ({ query, fallback, className }: QueryErrorProps) => (
  <div
    className={cn(
      "flex items-center gap-3 rounded-md border border-destructive/20 bg-destructive/10 p-3",
      className,
    )}
    role="alert"
  >
    <AlertTriangle
      aria-hidden="true"
      className="h-5 w-5 shrink-0 text-destructive"
    />
    <p className="min-w-0 flex-1 break-words text-sm text-destructive">
      {requestErrorMessage(query.error, fallback)}
    </p>
    {/* refetch() runs even a disabled query, which would skip the
        permission gate its hook applies, so a disabled query gets no Retry. */}
    {query.isEnabled && (
      <Button
        className="shrink-0"
        disabled={query.isFetching}
        onClick={() => void query.refetch()}
        size="sm"
        type="button"
        variant="outline"
      >
        {query.isFetching && (
          <Loader2
            aria-hidden="true"
            className="mr-2 h-3.5 w-3.5 animate-spin"
          />
        )}
        Retry
      </Button>
    )}
  </div>
);

export default QueryError;
