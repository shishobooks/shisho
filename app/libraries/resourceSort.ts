import type { SortConfig } from "@/libraries/sortSpec";

/**
 * Sort fields for the resource lists (tags, genres, series, people,
 * publishers). Each list mirrors its package's `*SortFields` map in Go
 * (pkg/tags/service.go and its siblings); the tokens are the constants in
 * pkg/sortspec/resource.go. The server rejects a field its list does not
 * accept.
 */
const resourceSort = <F extends string>(
  labels: Record<"name" | F, string>,
): SortConfig<"name" | F> => ({
  fields: Object.keys(labels) as ("name" | F)[],
  labels,
  defaultSort: [{ field: "name", direction: "asc" }],
});

/** Tags, genres, and series. */
export const BOOK_COUNT_RESOURCE_SORT = resourceSort({
  name: "Name",
  book_count: "Book count",
  date_added: "Date added",
});

export const PERSON_SORT = resourceSort({
  name: "Name",
  authored_book_count: "Books authored",
  narrated_file_count: "Files narrated",
  date_added: "Date added",
});

/** File count includes the files of sub-publishers, as the list shows. */
export const PUBLISHER_SORT = resourceSort({
  name: "Name",
  file_count: "File count",
  date_added: "Date added",
});
