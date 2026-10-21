import ResourceList from "@/components/library/ResourceList";
import { useGenresList } from "@/hooks/queries/genres";
import { useResourceListState } from "@/hooks/useResourceListState";
import { BOOK_COUNT_RESOURCE_SORT } from "@/libraries/resourceSort";

const GenresList = () => {
  const state = useResourceListState(BOOK_COUNT_RESOURCE_SORT);
  const query = useGenresList(state.queryParams, {
    enabled: state.pageSizeResolved,
  });

  return (
    <ResourceList
      itemConfig={(genre) => {
        const bookCount = genre.book_count ?? 0;
        return {
          name: genre.name,
          aliases: genre.aliases,
          badges: [
            {
              count: bookCount,
              singular: "book",
              plural: "books",
            },
          ],
        };
      }}
      itemLabel="genres"
      linkTo={(genre, libraryId) =>
        `/libraries/${libraryId}/genres/${genre.id}`
      }
      query={query}
      searchLabel="Search genres"
      state={state}
      subtitle="Browse genres in your library"
      title="Genres"
    />
  );
};

export default GenresList;
