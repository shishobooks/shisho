import { useEffect, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";

import LibraryLayout from "@/components/library/LibraryLayout";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import PaginationFooter from "@/components/library/PaginationFooter";
import { SearchInput } from "@/components/library/SearchInput";
import { Badge } from "@/components/ui/badge";
import { usePeopleList, type PersonWithCounts } from "@/hooks/queries/people";
import { usePageTitle } from "@/hooks/usePageTitle";

const ITEMS_PER_PAGE = 24;

const PersonList = () => {
  usePageTitle("People");

  const { libraryId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);
  const searchQuery = searchParams.get("search") ?? "";
  const [debouncedSearch, setDebouncedSearch] = useState(searchQuery);

  const handleDebouncedSearchChange = (value: string) => {
    setDebouncedSearch(value);
    if (value !== searchQuery) {
      setSearchParams((prev) => {
        const newParams = new URLSearchParams(prev);
        if (value) {
          newParams.set("search", value);
        } else {
          newParams.delete("search");
        }
        newParams.set("page", "1");
        return newParams;
      });
    }
  };

  const limit = ITEMS_PER_PAGE;
  const offset = (currentPage - 1) * limit;

  const peopleQuery = usePeopleList({
    limit,
    offset,
    library_id: libraryId ? parseInt(libraryId, 10) : undefined,
    search: debouncedSearch || undefined,
  });

  // Track the search value that produced the currently displayed data
  const [confirmedSearch, setConfirmedSearch] = useState<string | null>(null);

  useEffect(() => {
    if (peopleQuery.isSuccess && !peopleQuery.isFetching) {
      setConfirmedSearch(debouncedSearch);
    }
  }, [peopleQuery.isSuccess, peopleQuery.isFetching, debouncedSearch]);

  // Data is stale if search changed but query hasn't completed yet
  const isStaleData =
    confirmedSearch !== null && debouncedSearch !== confirmedSearch;

  const totalPages = Math.ceil((peopleQuery.data?.total ?? 0) / ITEMS_PER_PAGE);

  const renderPersonItem = (person: PersonWithCounts) => {
    const totalWorks = person.authored_book_count + person.narrated_file_count;

    return (
      <Link
        className="flex items-center justify-between p-4 rounded-md border border-border hover:bg-muted/50 transition-colors"
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
    <LibraryLayout maxWidth="max-w-4xl">
      <div className="mb-6">
        <h1 className="text-2xl font-semibold mb-2">People</h1>
        <p className="text-muted-foreground">
          Authors, narrators, and other contributors
        </p>
      </div>

      <div className="mb-6">
        <SearchInput
          className="max-w-md"
          initialValue={searchQuery}
          onDebouncedChange={handleDebouncedSearchChange}
          placeholder="Search by name..."
        />
      </div>

      {(peopleQuery.isLoading || peopleQuery.isFetching || isStaleData) && (
        <LoadingSpinner />
      )}

      {peopleQuery.isSuccess && !peopleQuery.isFetching && !isStaleData && (
        <>
          {peopleQuery.data.total > 0 && (
            <div className="mb-4 text-sm text-muted-foreground">
              Showing {offset + 1}-
              {Math.min(offset + limit, peopleQuery.data.total)} of{" "}
              {peopleQuery.data.total} people
            </div>
          )}

          <div className="space-y-2 mb-6">
            {peopleQuery.data.people.length === 0 ? (
              <div className="text-center py-8 text-muted-foreground">
                {confirmedSearch
                  ? "No people found matching your search."
                  : "No people in this library yet."}
              </div>
            ) : (
              peopleQuery.data.people.map(renderPersonItem)
            )}
          </div>

          <PaginationFooter
            buildHref={(page) =>
              `?page=${page}${searchQuery ? `&search=${searchQuery}` : ""}`
            }
            currentPage={currentPage}
            totalPages={totalPages}
          />
        </>
      )}
    </LibraryLayout>
  );
};

export default PersonList;
