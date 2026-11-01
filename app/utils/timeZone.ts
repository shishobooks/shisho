// The browser's IANA time zone, or undefined when it doesn't report one it
// can use itself (some report "Etc/Unknown"), which the server would refuse.
export const browserTimeZone = (): string | undefined => {
  try {
    const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    if (!zone) return undefined;
    new Intl.DateTimeFormat("en-US", { timeZone: zone });
    return zone;
  } catch {
    return undefined;
  }
};

// Every IANA time zone the browser knows, with zone (the one chosen) among
// them even when the browser's list leaves it out, as some leave out UTC.
export const timeZoneOptions = (zone?: string | null): string[] => {
  // The project's TypeScript lib predates Intl.supportedValuesOf.
  const intl = Intl as typeof Intl & {
    supportedValuesOf?: (key: "timeZone") => string[];
  };
  const zones = intl.supportedValuesOf?.("timeZone") ?? [];
  const all = new Set<string>(zones);
  all.add("UTC");
  if (zone) all.add(zone);
  return [...all].sort((a, b) => a.localeCompare(b));
};
