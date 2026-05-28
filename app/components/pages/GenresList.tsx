import ResourceList from "@/components/library/ResourceList";
import { useGenresList } from "@/hooks/queries/genres";
import { useResourceListState } from "@/hooks/useResourceListState";
import type { Genre } from "@/types";

const GenresList = () => {
  const state = useResourceListState();
  const query = useGenresList(state.queryParams);

  return (
    <ResourceList<Genre>
      itemConfig={(genre) => {
        const bookCount = genre.book_count ?? 0;
        return {
          name: genre.name,
          aliases: (genre.aliases as unknown as string[]) ?? [],
          badges: [
            {
              label: bookCount === 1 ? "book" : "books",
              count: bookCount,
            },
          ],
        };
      }}
      itemLabel="genres"
      linkTo={(genre, libraryId) =>
        `/libraries/${libraryId}/genres/${genre.id}`
      }
      query={query}
      searchPlaceholder="Search genres..."
      state={state}
      subtitle="Browse genres in your library"
      title="Genres"
    />
  );
};

export default GenresList;
