import { ReactNode } from "react";
import { useSearchParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import PaginationFooter from "@/components/library/PaginationFooter";

interface GalleryProps<T> {
  items: T[];
  total: number;
  isLoading: boolean;
  isSuccess: boolean;
  itemsPerPage?: number;
  renderItem: (item: T) => ReactNode;
  itemLabel: string;
  emptyMessage?: string;
}

const Gallery = <T,>({
  items,
  total,
  isLoading,
  isSuccess,
  itemsPerPage = 20,
  renderItem,
  itemLabel,
  emptyMessage,
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

  if (isLoading) {
    return <LoadingSpinner />;
  }

  if (!isSuccess) {
    return <div>Error loading {itemLabel}</div>;
  }

  return (
    <>
      {total > 0 && (
        <div className="mb-4 text-sm text-muted-foreground">
          Showing {offset + 1}-{Math.min(offset + limit, total)} of {total}{" "}
          {itemLabel}
        </div>
      )}

      {total === 0 && (
        <div className="text-center py-8 text-muted-foreground">
          {emptyMessage ?? `No ${itemLabel} found.`}
        </div>
      )}

      <div className="flex flex-wrap gap-4 sm:gap-4 mb-6 md:mb-8">
        {items.map(renderItem)}
      </div>

      <PaginationFooter
        className="mb-8"
        currentPage={currentPage}
        onPageChange={handlePageChange}
        totalPages={totalPages}
      />
    </>
  );
};

export default Gallery;
