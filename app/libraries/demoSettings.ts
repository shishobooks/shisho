export const readDemoSettings = <T extends object>(
  key: string,
  serverDefaults: T,
): T => {
  let settings = serverDefaults;

  // Storage access can throw (blocked site data, private windows), and the
  // stored JSON can be corrupt. Either way the server defaults still work.
  try {
    const saved = localStorage.getItem(key);
    const parsed: unknown = saved ? JSON.parse(saved) : null;
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      settings = { ...serverDefaults, ...parsed };
    }
  } catch {
    settings = serverDefaults;
  }

  writeDemoSettings(key, settings);
  return settings;
};

export const mergeDemoSettings = <T extends object, U extends object>(
  current: T,
  update: U,
): T => {
  const definedEntries = Object.entries(update).filter(
    ([, value]) => value !== undefined,
  );
  return { ...current, ...Object.fromEntries(definedEntries) };
};

// A failed write (quota, blocked storage) is not an error for the caller: the
// query cache still holds the new value for this session.
export const writeDemoSettings = <T extends object>(
  key: string,
  settings: T,
) => {
  try {
    localStorage.setItem(key, JSON.stringify(settings));
  } catch {
    // Ignored on purpose; see above.
  }
};
