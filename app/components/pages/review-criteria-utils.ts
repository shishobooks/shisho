/**
 * Field-name fragments that should render in all-caps when humanizing.
 */
const ACRONYMS = new Set(["url"]);

/**
 * Convert a snake_case field name to a human-readable label.
 * e.g. "release_date" → "Release date", "url" → "URL", "source_url" → "Source URL"
 */
export function humanizeField(name: string): string {
  const words = name.split("_");
  return words
    .map((word, i) => {
      if (ACRONYMS.has(word)) return word.toUpperCase();
      if (i === 0) return word.charAt(0).toUpperCase() + word.slice(1);
      return word;
    })
    .join(" ");
}
