import { ReactNode } from "react";
import { useSearchParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";

interface GalleryProps<T> {
  items: T[];
  total: number;
  isLoading: boolean;
  isSuccess: boolean;
  itemsPerPage?: number;
  renderItem: (item: T) => ReactNode;
  itemLabel: string;
}

const Gallery = <T,>({
  items,
  total,
  isLoading,
  isSuccess,
  itemsPerPage = 20,
  renderItem,
  itemLabel,
}: GalleryProps<T>) => {
  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parseInt(searchParams.get("page") ?? "1", 10);

  const limit = itemsPerPage;
  const offset = (currentPage - 1) * limit;
  const totalPages = Math.ceil(total / itemsPerPage);

  const handlePageChange = (page: number) => {
    const newSearchParams = new URLSearchParams(searchParams);
    newSearchParams.set("page", page.toString());
    setSearchParams(newSearchParams);
  };

  const getPageNumbers = () => {
    const pages = [];
    const showPages = 5;

    let start = Math.max(1, currentPage - Math.floor(showPages / 2));
    const end = Math.min(totalPages, start + showPages - 1);

    if (end - start + 1 < showPages) {
      start = Math.max(1, end - showPages + 1);
    }

    for (let i = start; i <= end; i++) {
      pages.push(i);
    }

    return pages;
  };

  if (isLoading) {
    return <LoadingSpinner />;
  }

  if (!isSuccess) {
    return <div>Error loading {itemLabel}</div>;
  }

  return (
    <>
      <div className="mb-4 text-sm text-muted-foreground">
        Showing {offset + 1}-{Math.min(offset + limit, total)} of {total}{" "}
        {itemLabel}
      </div>

      <div className="flex flex-wrap gap-4 mb-8">{items.map(renderItem)}</div>

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

            {getPageNumbers()[getPageNumbers().length - 1] < totalPages && (
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
  );
};

export default Gallery;
