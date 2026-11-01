import { useCallback } from "react";
import { matchPath, useNavigate } from "react-router-dom";

import { useNavigationHistory } from "@/contexts/NavigationHistory";
import type { RemovedEntities } from "@/types";

// The detail pages a delete can remove, by the URL segment that names them.
const REMOVABLE_PAGES: Record<string, keyof RemovedEntities> = {
  series: "series_ids",
  people: "person_ids",
  genres: "genre_ids",
  tags: "tag_ids",
  publishers: "publisher_ids",
};

// Whether the page at `pathname` showed something the delete removed.
const showsRemoved = (pathname: string, removed?: RemovedEntities) => {
  const match = matchPath("/libraries/:libraryId/:kind/:id", pathname);
  const key = match?.params.kind && REMOVABLE_PAGES[match.params.kind];
  if (!removed || !key) return false;
  return removed[key].includes(Number(match.params.id));
};

/**
 * Leaves the pages of a book that was just deleted. Goes Back to the nearest
 * page that still exists, so the reader returns to the gallery page, filters,
 * or series they came from. It walks past the book's own pages (its detail,
 * file, and reader pages) and the page of any series, person, genre, tag, or
 * publisher in `removed`, the delete or rescan response's list of the book's
 * entities that are gone. With no such page it replaces the current entry
 * with the book's library.
 *
 * It does nothing once the reader has left the book's pages, as they can with
 * the browser's Back while a slow request runs.
 */
export const useLeaveDeletedBook = () => {
  const navigate = useNavigate();
  const history = useNavigationHistory();

  return useCallback(
    (book: { id: number; library_id: number }, removed: RemovedEntities) => {
      // Any library: the book may have moved since an earlier visit.
      const isBookPage = (pathname: string) =>
        matchPath("/libraries/:libraryId/books/:bookId/*", pathname)?.params
          .bookId === String(book.id);

      const current = history?.currentPathname();
      if (current !== undefined && !isBookPage(current)) return;

      const steps =
        history
          ?.previousPathnames()
          .findIndex((p) => !isBookPage(p) && !showsRemoved(p, removed)) ?? -1;
      if (steps >= 0) {
        navigate(-(steps + 1));
        return;
      }
      navigate(`/libraries/${book.library_id}`, { replace: true });
    },
    [history, navigate],
  );
};
