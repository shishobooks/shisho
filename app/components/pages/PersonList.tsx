import { useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import { usePeopleList, type PersonWithCounts } from "@/hooks/queries/people";
import { useDebounce } from "@/hooks/useDebounce";

const ITEMS_PER_PAGE = 24;

const PersonList = () => {
  const { libraryId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);
  const searchQuery = searchParams.get("search") ?? "";
  const [searchInput, setSearchInput] = useState(searchQuery);
  const debouncedSearch = useDebounce(searchInput, 300);

  const limit = ITEMS_PER_PAGE;
  const offset = (currentPage - 1) * limit;

  // Handle search input change with debounce
  const handleSearchChange = (value: string) => {
    setSearchInput(value);
    // Update URL after debounce
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

  const peopleQuery = usePeopleList({
    limit,
    offset,
    library_id: libraryId ? parseInt(libraryId, 10) : undefined,
    search: debouncedSearch || undefined,
  });

  const totalPages = Math.ceil((peopleQuery.data?.total ?? 0) / ITEMS_PER_PAGE);

  const renderPersonItem = (person: PersonWithCounts) => {
    const totalWorks = person.authored_book_count + person.narrated_file_count;

    return (
      <Link
        className="flex items-center justify-between p-4 rounded-lg border border-neutral-200 dark:border-neutral-700 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
        key={person.id}
        to={`/libraries/${libraryId}/people/${person.id}`}
      >
        <div className="flex-1">
          <div className="font-semibold text-lg">{person.name}</div>
          {person.sort_name !== person.name && (
            <div className="text-sm text-muted-foreground">
              {person.sort_name}
            </div>
          )}
        </div>
        <div className="flex gap-2">
          {person.authored_book_count > 0 && (
            <Badge variant="secondary">
              {person.authored_book_count} book
              {person.authored_book_count !== 1 ? "s" : ""} authored
            </Badge>
          )}
          {person.narrated_file_count > 0 && (
            <Badge variant="outline">
              {person.narrated_file_count} file
              {person.narrated_file_count !== 1 ? "s" : ""} narrated
            </Badge>
          )}
          {totalWorks === 0 && <Badge variant="outline">No works</Badge>}
        </div>
      </Link>
    );
  };

  return (
    <div>
      <TopNav />
      <div className="max-w-4xl w-full mx-auto px-6 py-8">
        <div className="mb-6">
          <h1 className="text-2xl font-semibold mb-2">People</h1>
          <p className="text-muted-foreground">
            Authors, narrators, and other contributors
          </p>
        </div>

        <div className="mb-6">
          <Input
            className="max-w-md"
            onChange={(e) => handleSearchChange(e.target.value)}
            placeholder="Search by name..."
            type="search"
            value={searchInput}
          />
        </div>

        {peopleQuery.isLoading && <LoadingSpinner />}

        {peopleQuery.isSuccess && (
          <>
            <div className="space-y-2 mb-6">
              {peopleQuery.data.people.length === 0 ? (
                <div className="text-center py-8 text-muted-foreground">
                  {searchQuery
                    ? "No people found matching your search."
                    : "No people in this library yet."}
                </div>
              ) : (
                peopleQuery.data.people.map(renderPersonItem)
              )}
            </div>

            {totalPages > 1 && (
              <Pagination>
                <PaginationContent>
                  <PaginationItem>
                    <PaginationPrevious
                      href={`?page=${Math.max(1, currentPage - 1)}${searchQuery ? `&search=${searchQuery}` : ""}`}
                    />
                  </PaginationItem>
                  {Array.from({ length: Math.min(5, totalPages) }, (_, i) => {
                    let pageNum;
                    if (totalPages <= 5) {
                      pageNum = i + 1;
                    } else if (currentPage <= 3) {
                      pageNum = i + 1;
                    } else if (currentPage >= totalPages - 2) {
                      pageNum = totalPages - 4 + i;
                    } else {
                      pageNum = currentPage - 2 + i;
                    }
                    return (
                      <PaginationItem key={pageNum}>
                        <PaginationLink
                          href={`?page=${pageNum}${searchQuery ? `&search=${searchQuery}` : ""}`}
                          isActive={pageNum === currentPage}
                        >
                          {pageNum}
                        </PaginationLink>
                      </PaginationItem>
                    );
                  })}
                  <PaginationItem>
                    <PaginationNext
                      href={`?page=${Math.min(totalPages, currentPage + 1)}${searchQuery ? `&search=${searchQuery}` : ""}`}
                    />
                  </PaginationItem>
                </PaginationContent>
              </Pagination>
            )}
          </>
        )}
      </div>
    </div>
  );
};

export default PersonList;
