import ResourceList from "@/components/library/ResourceList";
import { useTagsList } from "@/hooks/queries/tags";
import { useResourceListState } from "@/hooks/useResourceListState";
import type { Tag } from "@/types";

const TagsList = () => {
  const state = useResourceListState();
  const query = useTagsList(state.queryParams);

  return (
    <ResourceList<Tag>
      itemConfig={(tag) => {
        const bookCount = tag.book_count ?? 0;
        return {
          name: tag.name,
          aliases: tag.aliases as unknown as string[],
          badges: [
            {
              label: bookCount === 1 ? "book" : "books",
              count: bookCount,
            },
          ],
        };
      }}
      itemLabel="tags"
      linkTo={(tag, libraryId) => `/libraries/${libraryId}/tags/${tag.id}`}
      query={query}
      searchPlaceholder="Search tags..."
      state={state}
      subtitle="Browse tags in your library"
      title="Tags"
    />
  );
};

export default TagsList;
