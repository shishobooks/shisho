import ResourceList from "@/components/library/ResourceList";
import {
  usePublishersList,
  type PublisherListItem,
} from "@/hooks/queries/publishers";
import { useResourceListState } from "@/hooks/useResourceListState";

const PublishersList = () => {
  const state = useResourceListState();
  const query = usePublishersList(state.queryParams);

  return (
    <ResourceList<PublisherListItem>
      itemConfig={(publisher) => {
        const fileCount = publisher.file_count ?? 0;
        const descendantFileCount = publisher.descendant_file_count ?? 0;
        const descendantPublisherCount =
          publisher.descendant_publisher_count ?? 0;

        const badges = [
          {
            label: fileCount === 1 ? "file" : "files",
            count: fileCount,
          },
        ];

        if (descendantFileCount > 0) {
          badges.push({
            label: descendantFileCount === 1 ? "sub-file" : "sub-files",
            count: descendantFileCount,
          });
        }

        if (descendantPublisherCount > 0) {
          badges.push({
            label:
              descendantPublisherCount === 1
                ? "sub-publisher"
                : "sub-publishers",
            count: descendantPublisherCount,
          });
        }

        return {
          name: publisher.name,
          secondaryText: publisher.parent_name ?? undefined,
          aliases: publisher.aliases as unknown as string[],
          badges,
        };
      }}
      itemLabel="publishers"
      linkTo={(publisher, libraryId) =>
        `/libraries/${libraryId}/publishers/${publisher.id}`
      }
      query={query}
      searchPlaceholder="Search publishers..."
      state={state}
      subtitle="Browse publishers in your library"
      title="Publishers"
    />
  );
};

export default PublishersList;
