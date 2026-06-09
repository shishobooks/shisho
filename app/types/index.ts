export * from "./generated/models";
export {
  type AuthorInput,
  type DeleteBookResponse,
  type DeleteBooksPayload,
  type DeleteBooksResponse,
  type DeleteFileResponse,
  type IdentifierPayload,
  type ListBooksQuery,
  type MergeBooksPayload,
  type MergeBooksResponse,
  type MoveFilesPayload,
  type MoveFilesResponse,
  type ResyncBookResponse,
  type ResyncFileResponse,
  type ResyncPayload,
  type SeriesInput,
  type UpdateBookPayload,
  type UpdateFilePayload,
} from "./generated/books";
export * from "./generated/filesystem";
export * from "./generated/jobs";
export * from "./generated/joblogs";
export * from "./generated/logs";
export * from "./generated/libraries";
export * from "./generated/auth";
export * from "./generated/users";
export * from "./generated/roles";
export * from "./generated/search";
export * from "./generated/series";
export * from "./generated/people";
export {
  type GenreResponse,
  type ListGenresQuery,
  type UpdateGenrePayload,
  type MergeGenresPayload,
} from "./generated/genres";
export {
  type TagResponse,
  type ListTagBooksResponse,
  type ListTagsQuery,
  type UpdateTagPayload,
  type MergeTagsPayload,
} from "./generated/tags";
export {
  type PublisherResponse,
  type PublisherListItem,
  type ListPublishersResponse,
  type ListPublisherFilesResponse,
  type AncestorResponse,
  type ChildResponse,
  type ListPublishersQuery,
  type UpdatePublisherPayload,
  type MergePublishersPayload,
} from "./generated/publishers";
export * from "./generated/chapters";
export {
  type Response as AudnexusChaptersResponse,
  type Chapter as AudnexusChapter,
} from "./generated/audnexus";
export {
  type UserSettingsResponse,
  type UserSettingsPayload,
  type LibrarySettingsResponse,
  type UpdateLibrarySettingsPayload,
  type ReviewCriteriaResponse,
  type PutReviewCriteriaPayload,
} from "./generated/settings";
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
  type ListResponse,
  type ListResponsePermission,
  type ListListsResponse,
  type ListListBooksResponse,
  type RetrieveListResponse,
  type CheckVisibilityResponse,
  type ListTemplate,
} from "./generated/lists";

export interface ResourceListResponse<T> {
  items: T[];
  total: number;
}
