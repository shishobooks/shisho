import ResourceList from "@/components/library/ResourceList";
import { useTagsList } from "@/hooks/queries/tags";
import { useResourceListState } from "@/hooks/useResourceListState";
import { BOOK_COUNT_RESOURCE_SORT } from "@/libraries/resourceSort";

const TagsList = () => {
  const state = useResourceListState(BOOK_COUNT_RESOURCE_SORT);
  const query = useTagsList(state.queryParams, {
    enabled: state.pageSizeResolved,
  });

  return (
    <ResourceList
      itemConfig={(tag) => {
        const bookCount = tag.book_count ?? 0;
        return {
          name: tag.name,
          aliases: tag.aliases,
          badges: [
            {
              count: bookCount,
              singular: "book",
              plural: "books",
            },
          ],
        };
      }}
      itemLabel="tags"
      linkTo={(tag, libraryId) => `/libraries/${libraryId}/tags/${tag.id}`}
      query={query}
      searchLabel="Search tags"
      state={state}
      subtitle="Browse tags in your library"
      title="Tags"
    />
  );
};

export default TagsList;
