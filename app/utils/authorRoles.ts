/**
 * Maps a canonical author role string (as stored on the Author model, e.g.
 * sourced from CBZ ComicInfo.xml creator fields) to its display label.
 *
 * Returns null when the role is missing, so callers can conditionally render.
 */
export function getAuthorRoleLabel(
  role: string | undefined | null,
): string | null {
  if (!role) return null;
  const roleLabels: Record<string, string> = {
    writer: "Writer",
    penciller: "Penciller",
    inker: "Inker",
    colorist: "Colorist",
    letterer: "Letterer",
    cover_artist: "Cover Artist",
    editor: "Editor",
    translator: "Translator",
  };
  return roleLabels[role] || role;
}
