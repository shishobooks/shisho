export const readDemoSettings = <T extends object>(
  key: string,
  serverDefaults: T,
): T => {
  let settings = serverDefaults;
  const saved = localStorage.getItem(key);

  if (saved) {
    try {
      const parsed: unknown = JSON.parse(saved);
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
        settings = { ...serverDefaults, ...parsed };
      }
    } catch {
      settings = serverDefaults;
    }
  }

  localStorage.setItem(key, JSON.stringify(settings));
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

export const writeDemoSettings = <T extends object>(
  key: string,
  settings: T,
) => {
  localStorage.setItem(key, JSON.stringify(settings));
};
