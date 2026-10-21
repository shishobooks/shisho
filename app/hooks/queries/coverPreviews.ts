import type { QueryClient } from "@tanstack/react-query";

import { QueryKey as GenresQueryKey } from "./genres";
import { QueryKey as PeopleQueryKey } from "./people";
import { QueryKey as PublishersQueryKey } from "./publishers";
import { QueryKey as TagsQueryKey } from "./tags";

/**
 * Refreshes the lists whose entries show book covers (`preview_books`), whose
 * data carries each cover's cache key. Call it wherever a book's cover can
 * change, or the lists keep showing the old image.
 */
export const invalidateCoverPreviewLists = (queryClient: QueryClient) => {
  for (const key of [
    PeopleQueryKey.ListPeople,
    GenresQueryKey.ListGenres,
    TagsQueryKey.ListTags,
    PublishersQueryKey.ListPublishers,
  ]) {
    queryClient.invalidateQueries({ queryKey: [key] });
  }
};
