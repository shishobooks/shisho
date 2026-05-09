import { Link, useSearchParams } from "react-router-dom";

import FileCoverThumbnail from "@/components/library/FileCoverThumbnail";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import PaginationFooter from "@/components/library/PaginationFooter";
import { Badge } from "@/components/ui/badge";
import type { File, ResourceListResponse } from "@/types";
import { formatDuration, getFilename } from "@/utils/format";

export const FILE_LIST_ITEMS_PER_PAGE = 50;

interface FileListQuery {
  data: ResourceListResponse<File> | undefined;
  isLoading: boolean;
  isSuccess: boolean;
  dataUpdatedAt: number;
}

interface FileListSectionProps {
  libraryId: string;
  query: FileListQuery;
  title: string;
  emptyMessage?: string;
  /** URL search param name for pagination (defaults to "page") */
  pageParam?: string;
}

function FileMetaInfo({ file }: { file: File }) {
  if (
    file.file_type === "m4b" &&
    file.audiobook_duration_seconds != null &&
    file.audiobook_duration_seconds > 0
  ) {
    return (
      <span className="text-xs text-muted-foreground">
        {formatDuration(file.audiobook_duration_seconds)}
      </span>
    );
  }

  if (
    (file.file_type === "cbz" || file.file_type === "pdf") &&
    file.page_count != null &&
    file.page_count > 0
  ) {
    return (
      <span className="text-xs text-muted-foreground">
        {file.page_count} page{file.page_count !== 1 ? "s" : ""}
      </span>
    );
  }

  return null;
}

export function FileListSection({
  libraryId,
  query,
  title,
  emptyMessage,
  pageParam = "page",
}: FileListSectionProps) {
  const [searchParams, setSearchParams] = useSearchParams();

  const currentPage = parseInt(searchParams.get(pageParam) ?? "1", 10);
  const offset = (currentPage - 1) * FILE_LIST_ITEMS_PER_PAGE;
  const total = query.data?.total ?? 0;
  const totalPages = Math.ceil(total / FILE_LIST_ITEMS_PER_PAGE);

  const handlePageChange = (page: number) => {
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      if (page === 1) {
        params.delete(pageParam);
      } else {
        params.set(pageParam, page.toString());
      }
      return params;
    });
  };

  if (query.isLoading) {
    return (
      <section className="mb-10">
        <h2 className="text-xl font-semibold mb-4">{title}</h2>
        <LoadingSpinner />
      </section>
    );
  }

  if (total === 0) {
    return (
      <section className="mb-10">
        <h2 className="text-xl font-semibold mb-4">{title}</h2>
        <div className="text-center py-8 text-muted-foreground">
          {emptyMessage ?? "No files found."}
        </div>
      </section>
    );
  }

  return (
    <section className="mb-10">
      <h2 className="text-xl font-semibold mb-4">{title}</h2>

      <div className="mb-4 text-sm text-muted-foreground">
        Showing {offset + 1}-
        {Math.min(offset + FILE_LIST_ITEMS_PER_PAGE, total)} of {total} files
      </div>

      <div className="space-y-2 mb-6 md:mb-8">
        {query.data?.items.map((file) => (
          <Link
            className="flex items-center gap-3 rounded-md border p-2 hover:bg-muted/50 transition-colors"
            key={file.id}
            to={`/libraries/${libraryId}/books/${file.book_id}`}
          >
            <div className="w-12 shrink-0">
              <FileCoverThumbnail
                cacheKey={query.dataUpdatedAt}
                className="w-full"
                file={file}
                interactive={false}
              />
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium truncate">
                {file.name || getFilename(file.filepath)}
              </p>
              <p className="text-xs text-muted-foreground truncate">
                {file.book?.title ?? "Unknown Book"}
              </p>
            </div>
            <div className="flex items-center gap-2 shrink-0">
              <FileMetaInfo file={file} />
              <Badge variant="outline">{file.file_type?.toUpperCase()}</Badge>
            </div>
          </Link>
        ))}
      </div>

      <PaginationFooter
        className="mb-8"
        currentPage={currentPage}
        onPageChange={handlePageChange}
        totalPages={totalPages}
      />
    </section>
  );
}
