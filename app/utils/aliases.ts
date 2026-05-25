/**
 * Fold pending alias input into a committed alias list.
 *
 * Trims whitespace, ignores empty/whitespace-only input, and skips
 * case-insensitive duplicates of existing aliases.
 */
export function resolveAliases(
  committed: string[],
  pendingInput: string,
): string[] {
  const trimmed = pendingInput.trim();
  if (!trimmed) return committed;
  if (committed.some((a) => a.toLowerCase() === trimmed.toLowerCase())) {
    return committed;
  }
  return [...committed, trimmed];
}
