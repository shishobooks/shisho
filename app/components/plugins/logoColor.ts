// 10-entry palette of saturated but not-harsh hues. Hex values chosen to render
// legibly on the muted 5%-white backdrop used by <PluginLogo />.
export const PLUGIN_LOGO_PALETTE = [
  "#7a5cff", // violet
  "#2ea4ff", // blue
  "#13b981", // green
  "#e2a42c", // amber
  "#e36a2c", // orange
  "#e8487f", // pink
  "#9f4dd8", // purple
  "#2ac3a2", // teal
  "#5a6bff", // indigo
  "#d85555", // red
] as const;

export const derivePluginInitials = (id: string): string => {
  if (!id) return "?";
  const hyphenIdx = id.indexOf("-");
  if (hyphenIdx > 0 && hyphenIdx < id.length - 1) {
    return (id[0] + id[hyphenIdx + 1]).toUpperCase();
  }
  if (id.length >= 2) {
    return id.slice(0, 2).toUpperCase();
  }
  return id.toUpperCase();
};

const hashString = (s: string): number => {
  // djb2-style hash; `>>> 0` coerces to an unsigned 32-bit int so the result is
  // always non-negative (avoids the `Math.abs(INT32_MIN) === INT32_MIN` pitfall).
  let h = 5381;
  for (let i = 0; i < s.length; i++) {
    h = (h * 33) ^ s.charCodeAt(i);
  }
  return h >>> 0;
};

export const getPluginFallbackColor = (scope: string, id: string): string => {
  const idx = hashString(`${scope}/${id}`) % PLUGIN_LOGO_PALETTE.length;
  return PLUGIN_LOGO_PALETTE[idx];
};
