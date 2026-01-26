import { useEffect } from "react";

const APP_NAME = "Shisho";

/**
 * Hook that sets the document title with the app name suffix.
 *
 * @param title - The page-specific title. Pass undefined or empty string to show just the app name.
 *
 * @example
 * // Shows "Books - Shisho" as the document title
 * usePageTitle("Books");
 *
 * @example
 * // Shows "The Great Gatsby - Shisho" as the document title
 * usePageTitle(book?.name);
 *
 * @example
 * // Shows just "Shisho" as the document title
 * usePageTitle();
 */
export function usePageTitle(title?: string): void {
  useEffect(() => {
    const previousTitle = document.title;
    document.title = title ? `${title} - ${APP_NAME}` : APP_NAME;

    return () => {
      document.title = previousTitle;
    };
  }, [title]);
}
