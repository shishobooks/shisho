export function extractSubtitleFromTitle(
  title: string,
): { title: string; subtitle: string } | null {
  const idx = title.indexOf(":");
  if (idx === -1) return null;
  const newTitle = title.slice(0, idx).trim();
  const newSubtitle = title.slice(idx + 1).trim();
  if (!newTitle || !newSubtitle) return null;
  return { title: newTitle, subtitle: newSubtitle };
}
