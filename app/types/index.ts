export * from "./generated/models";
export {
  type AuthorInput,
  type IdentifierPayload,
  type MergeBooksPayload,
  type MergeBooksResponse,
  type MoveFilesPayload,
  type MoveFilesResponse,
  type ResyncPayload,
  type SeriesInput,
  type UpdateBookPayload,
  type UpdateFilePayload,
  type ListBooksQuery as GeneratedListBooksQuery,
} from "./generated/books";

// Extended ListBooksQuery with ids filter for bulk operations
export interface ListBooksQuery {
  limit?: number;
  offset?: number;
  library_id?: number;
  series_id?: number;
  search?: string;
  file_types?: string[];
  genre_ids?: number[];
  tag_ids?: number[];
  language?: string;
  ids?: number[];
  sort?: string;
  reviewed_filter?: string; // "" or "all" = all books, "needs_review", "reviewed"
}
export * from "./generated/filesystem";
export * from "./generated/jobs";
export * from "./generated/joblogs";
export * from "./generated/libraries";
export * from "./generated/auth";
export * from "./generated/users";
export * from "./generated/roles";
export * from "./generated/search";
export * from "./generated/series";
export * from "./generated/people";
export {
  type ListGenresQuery,
  type UpdateGenrePayload,
  type MergeGenresPayload,
} from "./generated/genres";
export {
  type ListTagsQuery,
  type UpdateTagPayload,
  type MergeTagsPayload,
} from "./generated/tags";
export {
  type ListPublishersQuery,
  type UpdatePublisherPayload,
  type MergePublishersPayload,
} from "./generated/publishers";
export {
  type ListImprintsQuery,
  type UpdateImprintPayload,
  type MergeImprintsPayload,
} from "./generated/imprints";
export * from "./generated/chapters";
export {
  type CreateListPayload,
  type UpdateListPayload,
  type AddBooksPayload,
  type RemoveBooksPayload,
  type ReorderBooksPayload,
  type CreateSharePayload,
  type UpdateSharePayload,
  type UpdateBookListsPayload,
  type ListListsQuery,
  type ListBooksQuery as ListBooksInListQuery,
} from "./generated/lists";

export type { UserSettings } from "@/hooks/queries/settings";

export interface ResourceListResponse<T> {
  items: T[];
  total: number;
}

// Delete operation types
// TODO: Move these to validators.go and regenerate via tygo
export interface DeleteBookResponse {
  files_deleted: number;
}

export interface DeleteFileResponse {
  book_deleted: boolean;
}

export interface DeleteBooksPayload {
  book_ids: number[];
}

export interface DeleteBooksResponse {
  books_deleted: number;
  files_deleted: number;
}
