import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import { useMediaQuery } from "@/hooks/useMediaQuery";

interface PaginationFooterProps {
  currentPage: number;
  totalPages: number;
  onPageChange?: (page: number) => void;
  buildHref?: (page: number) => string;
  className?: string;
}

function getPageNumbers(
  currentPage: number,
  totalPages: number,
  showPages: number,
): number[] {
  const pages: number[] = [];
  let start = Math.max(1, currentPage - Math.floor(showPages / 2));
  const end = Math.min(totalPages, start + showPages - 1);

  if (end - start + 1 < showPages) {
    start = Math.max(1, end - showPages + 1);
  }

  for (let i = start; i <= end; i++) {
    pages.push(i);
  }

  return pages;
}

const PaginationFooter = ({
  currentPage,
  totalPages,
  onPageChange,
  buildHref,
  className,
}: PaginationFooterProps) => {
  const isSmallScreen = !useMediaQuery("(min-width: 640px)");
  const showPages = isSmallScreen ? 3 : 5;
  const pages = getPageNumbers(currentPage, totalPages, showPages);

  if (totalPages <= 1) return null;

  const linkProps = (page: number) => ({
    ...(onPageChange ? { onClick: () => onPageChange(page) } : {}),
    ...(buildHref ? { href: buildHref(page) } : {}),
  });

  return (
    <Pagination className={className}>
      <PaginationContent>
        <PaginationItem>
          <PaginationPrevious
            className={
              currentPage <= 1
                ? "pointer-events-none opacity-50"
                : "cursor-pointer"
            }
            {...linkProps(currentPage - 1)}
          />
        </PaginationItem>

        {pages[0] > 1 && (
          <>
            <PaginationItem>
              <PaginationLink className="cursor-pointer" {...linkProps(1)}>
                1
              </PaginationLink>
            </PaginationItem>
            {pages[0] > 2 && (
              <PaginationItem>
                <PaginationEllipsis />
              </PaginationItem>
            )}
          </>
        )}

        {pages.map((page) => (
          <PaginationItem key={page}>
            <PaginationLink
              className="cursor-pointer"
              isActive={page === currentPage}
              {...linkProps(page)}
            >
              {page}
            </PaginationLink>
          </PaginationItem>
        ))}

        {pages[pages.length - 1] < totalPages && (
          <>
            {pages[pages.length - 1] < totalPages - 1 && (
              <PaginationItem>
                <PaginationEllipsis />
              </PaginationItem>
            )}
            <PaginationItem>
              <PaginationLink
                className="cursor-pointer"
                {...linkProps(totalPages)}
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
            {...linkProps(currentPage + 1)}
          />
        </PaginationItem>
      </PaginationContent>
    </Pagination>
  );
};

export default PaginationFooter;
