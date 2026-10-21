import ResourceList from "@/components/library/ResourceList";
import { usePublishersList } from "@/hooks/queries/publishers";
import { useResourceListState } from "@/hooks/useResourceListState";
import { PUBLISHER_SORT } from "@/libraries/resourceSort";

const PublishersList = () => {
  const state = useResourceListState(PUBLISHER_SORT);
  const query = usePublishersList(state.queryParams, {
    enabled: state.pageSizeResolved,
  });

  return (
    <ResourceList
      itemConfig={(publisher) => {
        const fileCount = publisher.file_count ?? 0;
        const descendantFileCount = publisher.descendant_file_count ?? 0;
        const descendantPublisherCount =
          publisher.descendant_publisher_count ?? 0;

        const badges = [
          {
            count: fileCount,
            singular: "file",
            plural: "files",
          },
        ];

        if (descendantFileCount > 0) {
          badges.push({
            count: descendantFileCount,
            singular: "sub-file",
            plural: "sub-files",
          });
        }

        if (descendantPublisherCount > 0) {
          badges.push({
            count: descendantPublisherCount,
            singular: "sub-publisher",
            plural: "sub-publishers",
          });
        }

        return {
          name: publisher.name,
          secondaryText: publisher.parent_name ?? undefined,
          aliases: publisher.aliases,
          previewBooks: publisher.preview_books,
          badges,
        };
      }}
      itemLabel="publishers"
      linkTo={(publisher, libraryId) =>
        `/libraries/${libraryId}/publishers/${publisher.id}`
      }
      query={query}
      searchLabel="Search publishers"
      state={state}
      subtitle="Browse publishers in your library"
      title="Publishers"
    />
  );
};

export default PublishersList;
