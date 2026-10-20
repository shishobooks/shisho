/**
 * The in-app path a `?redirect=` param names, or `null` when it names
 * anything else. Only a path on this site is followed: `//host` and `/\host`
 * are protocol-relative URLs browsers send to another site.
 */
export function safeRedirectPath(value: string | null): string | null {
  if (!value || !value.startsWith("/")) {
    return null;
  }
  if (value.startsWith("//") || value.startsWith("/\\")) {
    return null;
  }
  return value;
}
