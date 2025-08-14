import { uniqBy } from "lodash";
import { Link, useSearchParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import { useBooks } from "@/hooks/queries/books";

const ITEMS_PER_PAGE = 20;

const Home = () => {
  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);

  // Calculate pagination parameters
  const limit = ITEMS_PER_PAGE;
  const offset = (currentPage - 1) * limit;

  const booksQuery = useBooks({ limit, offset });

  // Calculate pagination info
  const totalBooks = booksQuery.data?.total ?? 0;
  const totalPages = Math.ceil(totalBooks / ITEMS_PER_PAGE);

  const handlePageChange = (page: number) => {
    const newSearchParams = new URLSearchParams(searchParams);
    newSearchParams.set("page", page.toString());
    setSearchParams(newSearchParams);
  };

  // Generate page numbers for pagination
  const getPageNumbers = () => {
    const pages = [];
    const showPages = 5; // Show 5 page numbers

    let start = Math.max(1, currentPage - Math.floor(showPages / 2));
    const end = Math.min(totalPages, start + showPages - 1);

    // Adjust start if we're near the end
    if (end - start + 1 < showPages) {
      start = Math.max(1, end - showPages + 1);
    }

    for (let i = start; i <= end; i++) {
      pages.push(i);
    }

    return pages;
  };

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full p-8 m-auto">
        {booksQuery.isLoading ? (
          <LoadingSpinner />
        ) : !booksQuery.isSuccess ? (
          <div>error</div>
        ) : (
          <>
            {/* Books count */}
            <div className="mb-6 text-sm text-muted-foreground">
              Showing {offset + 1}-{Math.min(offset + limit, totalBooks)} of{" "}
              {totalBooks} books
            </div>

            {/* Books grid */}
            <div className="flex flex-wrap gap-4 p-4 mb-8">
              {booksQuery.data.books.map((book) => (
                <div
                  className="w-32"
                  key={book.id}
                  title={`${book.title}${book.authors ? `\nBy ${book.authors.map((a) => a.name).join(", ")}` : ""}`}
                >
                  <Link
                    className="group cursor-pointer"
                    to={`/books/${book.id}`}
                  >
                    <img
                      alt={`${book.title} Cover`}
                      className="h-48 object-cover rounded-sm border-neutral-300 dark:border-neutral-600 border-1"
                      onError={(e) => {
                        (e.target as HTMLImageElement).style.display = "none";
                        (
                          e.target as HTMLImageElement
                        ).nextElementSibling!.textContent = "no cover";
                      }}
                      src={`/api/books/${book.id}/cover`}
                    />
                    <div className="mt-2 group-hover:underline font-bold line-clamp-2 w-32">
                      {book.title}
                    </div>
                  </Link>
                  {book.authors && (
                    <div className="mt-1 text-sm line-clamp-2 text-neutral-500 dark:text-neutral-500">
                      {book.authors.map((a) => a.name).join(", ")}
                    </div>
                  )}
                  {book.files && (
                    <div className="mt-2 flex gap-2 text-sm">
                      {uniqBy(book.files, "file_type").map((f) => (
                        <Badge
                          className="uppercase"
                          key={f.id}
                          variant="subtle"
                        >
                          {f.file_type}
                        </Badge>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>

            {/* Pagination */}
            {totalPages > 1 && (
              <Pagination className="mb-8">
                <PaginationContent>
                  <PaginationItem>
                    <PaginationPrevious
                      className={
                        currentPage <= 1
                          ? "pointer-events-none opacity-50"
                          : "cursor-pointer"
                      }
                      onClick={() => handlePageChange(currentPage - 1)}
                    />
                  </PaginationItem>

                  {/* First page */}
                  {getPageNumbers()[0] > 1 && (
                    <>
                      <PaginationItem>
                        <PaginationLink
                          className="cursor-pointer"
                          onClick={() => handlePageChange(1)}
                        >
                          1
                        </PaginationLink>
                      </PaginationItem>
                      {getPageNumbers()[0] > 2 && (
                        <PaginationItem>
                          <PaginationEllipsis />
                        </PaginationItem>
                      )}
                    </>
                  )}

                  {/* Page numbers */}
                  {getPageNumbers().map((page) => (
                    <PaginationItem key={page}>
                      <PaginationLink
                        className="cursor-pointer"
                        isActive={page === currentPage}
                        onClick={() => handlePageChange(page)}
                      >
                        {page}
                      </PaginationLink>
                    </PaginationItem>
                  ))}

                  {/* Last page */}
                  {getPageNumbers()[getPageNumbers().length - 1] <
                    totalPages && (
                    <>
                      {getPageNumbers()[getPageNumbers().length - 1] <
                        totalPages - 1 && (
                        <PaginationItem>
                          <PaginationEllipsis />
                        </PaginationItem>
                      )}
                      <PaginationItem>
                        <PaginationLink
                          className="cursor-pointer"
                          onClick={() => handlePageChange(totalPages)}
                        >
                          {totalPages}
                        </PaginationLink>
                      </PaginationItem>
                    </>
                  )}

                  <PaginationItem>
                    <PaginationNext
                      className={
                        currentPage >= totalPages
                          ? "pointer-events-none opacity-50"
                          : "cursor-pointer"
                      }
                      onClick={() => handlePageChange(currentPage + 1)}
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

export default Home;
