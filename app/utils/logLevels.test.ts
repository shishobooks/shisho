import { describe, expect, it } from "vitest";

import {
  LogLevelDebug,
  LogLevelError,
  LogLevelFatal,
  LogLevelInfo,
  LogLevelWarn,
  type LogLevel,
} from "@/types";

import { LOG_LEVEL_COLORS } from "./logLevels";

describe("LOG_LEVEL_COLORS", () => {
  // A palette color picked for one theme is unreadable in the other, such as
  // text-yellow-400 on a white page, so each one comes with a dark: variant.
  // Debug uses theme tokens, which follow the theme already.
  it.each<LogLevel>([LogLevelInfo, LogLevelWarn, LogLevelError, LogLevelFatal])(
    "gives %s a light color and a dark: one",
    (level) => {
      const classes = LOG_LEVEL_COLORS[level].split(" ");
      expect(classes.some((c) => /^text-\w+-[6-9]00$/.test(c))).toBe(true);
      expect(classes.some((c) => c.startsWith("dark:text-"))).toBe(true);
    },
  );

  it("styles debug with theme tokens", () => {
    expect(LOG_LEVEL_COLORS[LogLevelDebug]).toBe(
      "bg-muted text-muted-foreground",
    );
  });
});
