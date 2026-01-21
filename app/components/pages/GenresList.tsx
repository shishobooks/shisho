import { useEffect, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";

import LibraryLayout from "@/components/library/LibraryLayout";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { useGenresList } from "@/hooks/queries/genres";
import { useDebounce } from "@/hooks/useDebounce";
import type { Genre } from "@/types";

const ITEMS_PER_PAGE = 50;

const GenresList = () => {
  const { libraryId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);
  const searchQuery = searchParams.get("search") ?? "";

  const [searchInput, setSearchInput] = useState(searchQuery);
  const debouncedSearch = useDebounce(searchInput, 300);

  // Sync searchInput with URL when searchQuery changes
  useEffect(() => {
    setSearchInput(searchQuery);
  }, [searchQuery]);

  const limit = ITEMS_PER_PAGE;
  const offset = (currentPage - 1) * limit;

  // Handle search input change
  const handleSearchChange = (value: string) => {
    setSearchInput(value);
    setTimeout(() => {
      if (value !== searchQuery) {
        const newParams = new URLSearchParams(searchParams);
        if (value) {
          newParams.set("search", value);
        } else {
          newParams.delete("search");
        }
        newParams.set("page", "1");
        setSearchParams(newParams);
      }
    }, 300);
  };

  const genresQuery = useGenresList({
    limit,
    offset,
    library_id: libraryId ? parseInt(libraryId, 10) : undefined,
    search: debouncedSearch || undefined,
  });

  const total = genresQuery.data?.total ?? 0;
  const totalPages = Math.ceil(total / ITEMS_PER_PAGE);

  const handlePageChange = (page: number) => {
    const newParams = new URLSearchParams(searchParams);
    newParams.set("page", page.toString());
    setSearchParams(newParams);
  };

  const renderGenreItem = (genre: Genre) => {
    const bookCount = genre.book_count ?? 0;

    return (
      <Link
        className="flex items-center justify-between p-3 rounded-md hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
        key={genre.id}
        to={`/libraries/${libraryId}/genres/${genre.id}`}
      >
        <span className="font-medium">{genre.name}</span>
        <Badge variant="secondary">
          {bookCount} book{bookCount !== 1 ? "s" : ""}
        </Badge>
      </Link>
    );
  };

  return (
    <LibraryLayout maxWidth="max-w-3xl">
      <div className="mb-6">
        <h1 className="text-2xl font-semibold mb-2">Genres</h1>
        <p className="text-muted-foreground">Browse genres in your library</p>
      </div>

      <div className="mb-6">
        <Input
          className="max-w-xs"
          onChange={(e) => handleSearchChange(e.target.value)}
          placeholder="Search genres..."
          type="search"
          value={searchInput}
        />
      </div>

      {genresQuery.isLoading && (
        <div className="text-muted-foreground">Loading...</div>
      )}

      {genresQuery.isSuccess && genresQuery.data.genres.length === 0 && (
        <div className="text-muted-foreground">No genres found</div>
      )}

      {genresQuery.isSuccess && genresQuery.data.genres.length > 0 && (
        <div className="space-y-1">
          {genresQuery.data.genres.map(renderGenreItem)}
        </div>
      )}

      {totalPages > 1 && (
        <div className="mt-6 flex justify-center gap-2">
          <button
            className="px-3 py-1 rounded-md border disabled:opacity-50"
            disabled={currentPage <= 1}
            onClick={() => handlePageChange(currentPage - 1)}
          >
            Previous
          </button>
          <span className="px-3 py-1">
            Page {currentPage} of {totalPages}
          </span>
          <button
            className="px-3 py-1 rounded-md border disabled:opacity-50"
            disabled={currentPage >= totalPages}
            onClick={() => handlePageChange(currentPage + 1)}
          >
            Next
          </button>
        </div>
      )}
    </LibraryLayout>
  );
};

export default GenresList;
