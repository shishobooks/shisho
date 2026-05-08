import ResourceList from "@/components/library/ResourceList";
import { usePublishersList } from "@/hooks/queries/publishers";
import { useResourceListState } from "@/hooks/useResourceListState";
import type { Publisher } from "@/types";

const PublishersList = () => {
  const state = useResourceListState();
  const query = usePublishersList(state.queryParams);

  return (
    <ResourceList<Publisher>
      itemConfig={(publisher) => {
        const fileCount = publisher.file_count ?? 0;
        return {
          name: publisher.name,
          aliases: publisher.aliases.map((a) => a.name),
          badges: [
            {
              label: fileCount === 1 ? "file" : "files",
              count: fileCount,
            },
          ],
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
