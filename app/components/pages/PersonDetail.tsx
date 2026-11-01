import { useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";

import { BookGallerySection } from "@/components/library/BookGallerySection";
import {
  FILE_LIST_ITEMS_PER_PAGE,
  FileListSection,
} from "@/components/library/FileListSection";
import { ResourceDetail } from "@/components/library/ResourceDetail";
import { Badge } from "@/components/ui/badge";
import { useBooks } from "@/hooks/queries/books";
import {
  useDeletePerson,
  useMergePerson,
  usePeopleList,
  usePerson,
  usePersonNarratedFiles,
  useUpdatePerson,
} from "@/hooks/queries/people";
import { useCan } from "@/hooks/useCan";
import { useDebounce } from "@/hooks/useDebounce";
import { useGallerySizeParam } from "@/hooks/useGallerySizeParam";
import { usePageTitle } from "@/hooks/usePageTitle";
import { useSortParam } from "@/hooks/useSortParam";
import { firstFailedQuery } from "@/libraries/api";
import { parsePageParam } from "@/libraries/pagination";
import { BOOK_DETAIL_SORT } from "@/libraries/sortSpec";
import { formatCount, formatPersonCredits } from "@/utils/format";
import { writePermissionForEntity } from "@/utils/permissions";

const PersonDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const personId = id ? parseInt(id, 10) : undefined;

  const { itemsPerPage, offset, pageSizeResolved } = useGallerySizeParam();

  const filePage = parsePageParam(searchParams.get("filePage"));

  const personQuery = usePerson(personId);
  usePageTitle(personQuery.data?.name ?? "Person");

  const sort = useSortParam(BOOK_DETAIL_SORT);
  const authoredBooksQuery = useBooks(
    {
      person_id: personId,
      library_id: libraryId ? parseInt(libraryId, 10) : undefined,
      limit: itemsPerPage,
      offset,
      sort: sort.param,
    },
    {
      enabled: pageSizeResolved && Boolean(personId),
    },
  );

  const narratedFilesQuery = usePersonNarratedFiles(
    personId,
    {
      limit: FILE_LIST_ITEMS_PER_PAGE,
      offset: (filePage - 1) * FILE_LIST_ITEMS_PER_PAGE,
    },
    {
      enabled: Boolean(personId),
    },
  );

  const updatePersonMutation = useUpdatePerson();
  const mergePersonMutation = useMergePerson();
  const deletePersonMutation = useDeletePerson();

  const [mergeSearchRaw, setMergeSearchRaw] = useState("");
  const mergeSearch = useDebounce(mergeSearchRaw, 200, {
    immediate: (v) => v === "",
  });

  // Pre-fetch the merge candidates so the dialog opens without a loading
  // flash. Only a role that can merge gets the dialog.
  const canMerge = useCan(writePermissionForEntity("person"));
  const peopleListQuery = usePeopleList(
    {
      library_id: personQuery.data?.library_id,
      limit: 50,
      search: mergeSearch || undefined,
    },
    { enabled: canMerge && !!personQuery.data?.library_id },
  );

  const person = personQuery.data;
  const aliases = person?.aliases ?? [];
  const bookCount = person?.authored_book_count ?? 0;
  const narratedFileCount = person?.narrated_file_count ?? 0;

  // Show only the sections with something in them. The person's counts
  // decide on first render; the lists also count, because a scan refreshes
  // them but not the person, and a failed list shows so its error is seen.
  const showAuthored =
    bookCount > 0 ||
    (authoredBooksQuery.data?.total ?? 0) > 0 ||
    Boolean(firstFailedQuery(authoredBooksQuery));
  const showNarrated =
    narratedFileCount > 0 ||
    (narratedFilesQuery.data?.total ?? 0) > 0 ||
    Boolean(firstFailedQuery(narratedFilesQuery));

  const handleEdit = async (data: {
    name: string;
    sort_name?: string;
    aliases?: string[];
  }) => {
    if (!personId) return;
    return updatePersonMutation.mutateAsync({
      personId,
      payload: {
        name: data.name,
        sort_name: data.sort_name,
        aliases: data.aliases,
      },
    });
  };

  const handleMerge = async (sourceId: number) => {
    if (!personId) return;
    return mergePersonMutation.mutateAsync({
      targetId: personId,
      sourceId,
    });
  };

  const handleDelete = async () => {
    if (!personId) return;
    return deletePersonMutation.mutateAsync(
      { personId },
      { onSuccess: () => navigate(`/libraries/${libraryId}/people`) },
    );
  };

  return (
    <ResourceDetail
      aliases={aliases}
      bookCount={bookCount}
      breadcrumbItems={[
        { label: "People", to: `/libraries/${libraryId}/people` },
        { label: person?.name ?? "" },
      ]}
      countLabel={{ singular: "book authored", plural: "books authored" }}
      deleteConfig={{
        isPending: deletePersonMutation.isPending,
        onDelete: handleDelete,
        disabled:
          (person?.authored_book_count ?? 0) > 0 ||
          (person?.narrated_file_count ?? 0) > 0,
      }}
      editConfig={{
        isPending: updatePersonMutation.isPending,
        onSave: handleEdit,
        sortName: person?.sort_name,
        sortNameSource: person?.sort_name_source,
      }}
      entityId={personId!}
      entityType="person"
      extraBadges={
        narratedFileCount > 0 ? (
          <Badge variant="outline">
            {formatCount(narratedFileCount, "file")} narrated
          </Badge>
        ) : null
      }
      libraryId={libraryId!}
      mergeConfig={{
        entities:
          peopleListQuery.data?.items.map((p) => ({
            id: p.id,
            name: p.name,
            countText: formatPersonCredits(
              p.authored_book_count,
              p.narrated_file_count,
            ),
          })) ?? [],
        isLoadingEntities: peopleListQuery.isLoading,
        isSearchPending:
          mergeSearchRaw !== mergeSearch || peopleListQuery.isPlaceholderData,
        isPending: mergePersonMutation.isPending,
        onMerge: handleMerge,
        onSearch: setMergeSearchRaw,
      }}
      name={person?.name ?? ""}
      notFoundLabel="Person Not Found"
      query={personQuery}
      sortName={person?.sort_name}
    >
      {showAuthored && (
        <BookGallerySection
          emptyMessage="This person has no authored books."
          libraryId={libraryId!}
          query={authoredBooksQuery}
          sort={sort}
          title="Books Authored"
        />
      )}
      {showNarrated && (
        <FileListSection
          emptyMessage="This person has no narrated files."
          libraryId={libraryId!}
          pageParam="filePage"
          query={narratedFilesQuery}
          title="Files Narrated"
        />
      )}
      {!showAuthored && !showNarrated && (
        <div className="text-center py-8 text-muted-foreground">
          This person has no authored books or narrated files.
        </div>
      )}
    </ResourceDetail>
  );
};

export default PersonDetail;
