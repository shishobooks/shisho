import { useEffect, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";

import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { usePublishersList } from "@/hooks/queries/publishers";
import { useDebounce } from "@/hooks/useDebounce";
import type { Publisher } from "@/types";

const ITEMS_PER_PAGE = 50;

const PublishersList = () => {
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

  const publishersQuery = usePublishersList({
    limit,
    offset,
    library_id: libraryId ? parseInt(libraryId, 10) : undefined,
    search: debouncedSearch || undefined,
  });

  const total = publishersQuery.data?.total ?? 0;
  const totalPages = Math.ceil(total / ITEMS_PER_PAGE);

  const handlePageChange = (page: number) => {
    const newParams = new URLSearchParams(searchParams);
    newParams.set("page", page.toString());
    setSearchParams(newParams);
  };

  const renderPublisherItem = (publisher: Publisher) => {
    const fileCount = publisher.file_count ?? 0;

    return (
      <Link
        className="flex items-center justify-between p-3 rounded-md hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
        key={publisher.id}
        to={`/libraries/${libraryId}/publishers/${publisher.id}`}
      >
        <span className="font-medium">{publisher.name}</span>
        <Badge variant="secondary">
          {fileCount} file{fileCount !== 1 ? "s" : ""}
        </Badge>
      </Link>
    );
  };

  return (
    <div>
      <TopNav />
      <div className="max-w-3xl w-full mx-auto px-6 py-8">
        <div className="mb-6">
          <h1 className="text-2xl font-semibold mb-2">Publishers</h1>
          <p className="text-muted-foreground">
            Browse publishers in your library
          </p>
        </div>

        <div className="mb-6">
          <Input
            className="max-w-xs"
            onChange={(e) => handleSearchChange(e.target.value)}
            placeholder="Search publishers..."
            type="search"
            value={searchInput}
          />
        </div>

        {publishersQuery.isLoading && (
          <div className="text-muted-foreground">Loading...</div>
        )}

        {publishersQuery.isSuccess &&
          publishersQuery.data.publishers.length === 0 && (
            <div className="text-muted-foreground">No publishers found</div>
          )}

        {publishersQuery.isSuccess &&
          publishersQuery.data.publishers.length > 0 && (
            <div className="space-y-1">
              {publishersQuery.data.publishers.map(renderPublisherItem)}
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
      </div>
    </div>
  );
};

export default PublishersList;
