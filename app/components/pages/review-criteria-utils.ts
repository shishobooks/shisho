/**
 * Convert a snake_case field name to a human-readable label.
 * e.g. "release_date" → "Release date", "cover" → "Cover"
 */
export function humanizeField(name: string): string {
  const label = name.replace(/_/g, " ");
  return label.charAt(0).toUpperCase() + label.slice(1);
}
