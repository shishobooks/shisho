/**
 * Metadata entities that share the edit/merge/delete dialogs and the
 * ResourceDetail page. Books and files are not included: they have their own
 * detail pages and dialogs.
 */
export type EntityType = "person" | "series" | "genre" | "tag" | "publisher";
