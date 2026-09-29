import { useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";

import { BookGallerySection } from "@/components/library/BookGallerySection";
import {
  FILE_LIST_ITEMS_PER_PAGE,
  FileListSection,
} from "@/components/library/FileListSection";
import { ResourceDetail } from "@/components/library/ResourceDetail";
import { Badge } from "@/components/ui/badge";
import {
  useDeletePerson,
  useMergePerson,
  usePeopleList,
  usePerson,
  usePersonAuthoredBooks,
  usePersonNarratedFiles,
  useUpdatePerson,
} from "@/hooks/queries/people";
import { useCan } from "@/hooks/useCan";
import { useDebounce } from "@/hooks/useDebounce";
import { useGallerySizeParam } from "@/hooks/useGallerySizeParam";
import { usePageTitle } from "@/hooks/usePageTitle";
import { parsePageParam } from "@/libraries/pagination";
import { writePermissionForEntity } from "@/utils/permissions";

const PersonDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const personId = id ? parseInt(id, 10) : undefined;

  const {
    itemsPerPage,
    offset,
    settingsResolved: userSettingsResolved,
  } = useGallerySizeParam();

  const filePage = parsePageParam(searchParams.get("filePage"));

  const personQuery = usePerson(personId);
  usePageTitle(personQuery.data?.name ?? "Person");

  const authoredBooksQuery = usePersonAuthoredBooks(
    personId,
    {
      limit: itemsPerPage,
      offset,
    },
    {
      enabled: userSettingsResolved && Boolean(personId),
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

  const handleEdit = async (data: {
    name: string;
    sort_name?: string;
    aliases?: string[];
  }) => {
    if (!personId) return;
    await updatePersonMutation.mutateAsync({
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
    await mergePersonMutation.mutateAsync({
      targetId: personId,
      sourceId,
    });
  };

  const handleDelete = async () => {
    if (!personId) return;
    await deletePersonMutation.mutateAsync({ personId });
    navigate(`/libraries/${libraryId}/people`);
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
            {narratedFileCount}{" "}
            {narratedFileCount !== 1 ? "files narrated" : "file narrated"}
          </Badge>
        ) : null
      }
      isLoading={personQuery.isLoading}
      libraryId={libraryId!}
      mergeConfig={{
        entities:
          peopleListQuery.data?.items.map((p) => ({
            id: p.id,
            name: p.name,
            count: p.authored_book_count + p.narrated_file_count,
          })) ?? [],
        isLoadingEntities: peopleListQuery.isLoading,
        isPending: mergePersonMutation.isPending,
        onMerge: handleMerge,
        onSearch: setMergeSearchRaw,
      }}
      name={person?.name ?? ""}
      notFound={!personQuery.isLoading && (!personQuery.isSuccess || !person)}
      notFoundLabel="Person Not Found"
      sortName={person?.sort_name}
    >
      <BookGallerySection
        emptyMessage="This person has no authored books."
        libraryId={libraryId!}
        query={authoredBooksQuery}
        title="Books Authored"
      />
      <FileListSection
        emptyMessage="This person has no narrated files."
        libraryId={libraryId!}
        pageParam="filePage"
        query={narratedFilesQuery}
        title="Files Narrated"
      />
    </ResourceDetail>
  );
};

export default PersonDetail;
