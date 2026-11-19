import {
  LogLevelDebug,
  LogLevelError,
  LogLevelFatal,
  LogLevelInfo,
  LogLevelWarn,
  type LogLevel,
} from "@/types";

// The badge colors for each log level, shared by the server log viewer and
// a job's logs. The theme has no info or warning colors, so these are
// palette classes, each with a dark: variant: a color picked for one theme
// is unreadable in the other.
export const LOG_LEVEL_COLORS: Record<LogLevel, string> = {
  [LogLevelDebug]: "bg-muted text-muted-foreground",
  [LogLevelInfo]:
    "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  [LogLevelWarn]:
    "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
  [LogLevelError]:
    "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
  [LogLevelFatal]:
    "bg-red-200 text-red-900 dark:bg-red-900/50 dark:text-red-300",
};

/** The badge color for level, or info's for a level Shisho doesn't know. */
export const logLevelColor = (level: string): string =>
  LOG_LEVEL_COLORS[level as LogLevel] ?? LOG_LEVEL_COLORS[LogLevelInfo];
