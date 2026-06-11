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
  type ResyncMode,
  ResyncModeRefresh,
  ResyncModeReset,
  ResyncModeScan,
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
export {
  type SeriesResponse,
  type ListSeriesQuery,
  type UpdateSeriesPayload,
  type MergeSeriesPayload,
} from "./generated/series";
export {
  type PersonResponse,
  type ListPeopleQuery,
  type UpdatePersonPayload,
  type MergePeoplePayload,
} from "./generated/people";
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
  type ParsedAuthor,
  type ParsedChapter,
  type ParsedIdentifier,
  type ParsedMetadata,
} from "./generated/mediafile";
// Plugin API surface (ADR 0004 amendment). Aliased where the Go name differs
// from the established frontend name: the wire response for an available
// plugin is AvailablePluginResponse (the bare AvailablePlugin in generated
// plugins.ts is the repository-index entry, which the frontend never
// consumes), versions arrive annotated with compatibility, search results are
// EnrichSearchResult, and Capabilities is too generic a name to export bare.
// The Set*/Add* payload names gain a Plugin qualifier because the bare Go
// names (scoped by package plugins) are too generic for the global barrel.
export {
  type AddRepositoryPayload as AddPluginRepositoryPayload,
  type AnnotatedPluginVersion as PluginVersion,
  type AvailablePluginResponse as AvailablePlugin,
  type Capabilities as PluginCapabilities,
  type ConfigField,
  type ConfigFieldType,
  type ConfigSchema,
  type EnrichSearchResult as PluginSearchResult,
  type InstallPluginPayload,
  type LibraryPluginOrderPlugin,
  type LibraryPluginOrderResponse,
  type PluginApplyPayload,
  type PluginConfigResponse,
  type PluginSearchError,
  type PluginSearchPayload,
  type PluginSearchResponse,
  type PluginSearchSkipped,
  type SetFieldSettingsPayload as SetPluginFieldSettingsPayload,
  type SetLibraryOrderPayload as SetLibraryPluginOrderPayload,
  type SetOrderPayload as SetPluginOrderPayload,
  type SyncRepositoryResponse,
  type UpdatePluginPayload,
} from "./generated/plugins";
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
