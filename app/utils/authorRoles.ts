import {
  AuthorRoleColorist,
  AuthorRoleCoverArtist,
  AuthorRoleEditor,
  AuthorRoleInker,
  AuthorRoleLetterer,
  AuthorRolePenciller,
  AuthorRoleTranslator,
  AuthorRoleWriter,
} from "@/types";

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
    [AuthorRoleWriter]: "Writer",
    [AuthorRolePenciller]: "Penciller",
    [AuthorRoleInker]: "Inker",
    [AuthorRoleColorist]: "Colorist",
    [AuthorRoleLetterer]: "Letterer",
    [AuthorRoleCoverArtist]: "Cover Artist",
    [AuthorRoleEditor]: "Editor",
    [AuthorRoleTranslator]: "Translator",
  };
  return roleLabels[role] || role;
}

/**
 * Author role options for CBZ files, in the order they should appear in
 * dropdowns. Shared between BookEditDialog and IdentifyReviewForm so the
 * two stay in sync. Uses the tygo-generated `AuthorRoleX` constants so a
 * Go-side rename breaks at compile time instead of silently drifting.
 */
export const AUTHOR_ROLES = [
  { value: AuthorRoleWriter, label: "Writer" },
  { value: AuthorRolePenciller, label: "Penciller" },
  { value: AuthorRoleInker, label: "Inker" },
  { value: AuthorRoleColorist, label: "Colorist" },
  { value: AuthorRoleLetterer, label: "Letterer" },
  { value: AuthorRoleCoverArtist, label: "Cover Artist" },
  { value: AuthorRoleEditor, label: "Editor" },
  { value: AuthorRoleTranslator, label: "Translator" },
] as const;
