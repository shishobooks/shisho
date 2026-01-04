import { ArrowLeft, Edit, Upload } from "lucide-react";
import { useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";

import { BookEditDialog } from "@/components/library/BookEditDialog";
import { FileEditDialog } from "@/components/library/FileEditDialog";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { useBook, useUploadBookCover } from "@/hooks/queries/books";
import type { File } from "@/types";

const formatFileSize = (bytes: number): string => {
  const sizes = ["B", "KB", "MB", "GB"];
  if (bytes === 0) return "0 B";
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return Math.round((bytes / Math.pow(1024, i)) * 100) / 100 + " " + sizes[i];
};

const formatDuration = (seconds: number): string => {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  return `${minutes}m`;
};

const formatDate = (dateString: string): string => {
  return new Date(dateString).toLocaleDateString();
};

const BookDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const bookQuery = useBook(id);
  const uploadCoverMutation = useUploadBookCover();
  const coverInputRef = useRef<HTMLInputElement>(null);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [editingFile, setEditingFile] = useState<File | null>(null);

  const handleCoverUpload = async (
    event: React.ChangeEvent<HTMLInputElement>,
  ) => {
    const file = event.target.files?.[0];
    if (!file || !bookQuery.data) return;

    await uploadCoverMutation.mutateAsync({
      id: bookQuery.data.id,
      file,
    });

    // Reset the file input
    if (coverInputRef.current) {
      coverInputRef.current.value = "";
    }
  };

  if (bookQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (!bookQuery.isSuccess || !bookQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <div className="text-center">
            <h1 className="text-2xl font-semibold mb-4">Book Not Found</h1>
            <p className="text-muted-foreground mb-6">
              The book you're looking for doesn't exist or may have been
              removed.
            </p>
            <Button asChild>
              <Link to={`/libraries/${libraryId}`}>
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Home
              </Link>
            </Button>
          </div>
        </div>
      </div>
    );
  }

  const book = bookQuery.data;

  // Check if book cover is from an audiobook based on cover_image_path
  // If cover path contains "audiobook_cover", it should be square
  const isAudiobookCover = book.cover_image_path?.includes("audiobook_cover");
  const coverAspectRatio = isAudiobookCover ? "aspect-square" : "aspect-[2/3]";

  // Cache-busting parameter for cover images
  const coverCacheBuster = bookQuery.dataUpdatedAt;

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        <div className="mb-6">
          <Button asChild variant="ghost">
            <Link to={`/libraries/${libraryId}`}>
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Books
            </Link>
          </Button>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          {/* Book Cover */}
          <div className="lg:col-span-1">
            <div className={`${coverAspectRatio} w-full relative group`}>
              <img
                alt={`${book.title} Cover`}
                className="w-full h-full object-cover rounded-md border border-border"
                onError={(e) => {
                  (e.target as HTMLImageElement).style.display = "none";
                  (
                    e.target as HTMLImageElement
                  ).nextElementSibling!.textContent = "No cover available";
                }}
                src={`/api/books/${book.id}/cover?t=${coverCacheBuster}`}
              />
              <div className="hidden text-center text-muted-foreground"></div>
              {/* Cover upload overlay */}
              <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity rounded-md flex items-center justify-center">
                <input
                  accept="image/jpeg,image/png,image/webp"
                  className="hidden"
                  onChange={handleCoverUpload}
                  ref={coverInputRef}
                  type="file"
                />
                <Button
                  disabled={uploadCoverMutation.isPending}
                  onClick={() => coverInputRef.current?.click()}
                  size="sm"
                  variant="secondary"
                >
                  <Upload className="h-4 w-4 mr-2" />
                  {uploadCoverMutation.isPending ? "Uploading..." : "Replace"}
                </Button>
              </div>
            </div>
          </div>

          {/* Book Details */}
          <div className="lg:col-span-2 space-y-6">
            <div>
              <div className="flex items-start gap-3 mb-2">
                <h1 className="text-3xl font-semibold flex-1">{book.title}</h1>
                <Button
                  onClick={() => setEditDialogOpen(true)}
                  size="sm"
                  variant="outline"
                >
                  <Edit className="h-4 w-4 mr-2" />
                  Edit
                </Button>
              </div>
              {book.subtitle && (
                <p className="text-lg text-muted-foreground">{book.subtitle}</p>
              )}
            </div>

            <div className="space-y-6">
              {/* Authors */}
              {book.authors && book.authors.length > 0 && (
                <div>
                  <h3 className="font-semibold mb-2">Authors</h3>
                  <div className="flex flex-wrap gap-2">
                    {book.authors.map((author) => (
                      <Link
                        key={author.id}
                        to={`/libraries/${libraryId}/people/${author.person_id}`}
                      >
                        <Badge
                          className="cursor-pointer hover:bg-secondary/80"
                          variant="secondary"
                        >
                          {author.person?.name ?? "Unknown"}
                        </Badge>
                      </Link>
                    ))}
                  </div>
                </div>
              )}

              {/* Series */}
              {book.book_series && book.book_series.length > 0 && (
                <div>
                  <h3 className="font-semibold mb-2">Series</h3>
                  <div className="flex flex-wrap gap-3">
                    {book.book_series.map((bs) => (
                      <div className="flex items-center gap-2" key={bs.id}>
                        <Link
                          className="text-sm font-medium text-primary hover:text-primary/80 hover:underline dark:text-violet-300 dark:hover:text-violet-400"
                          to={`/libraries/${libraryId}/series/${bs.series_id}`}
                        >
                          {bs.series?.name ?? "Unknown Series"}
                        </Link>
                        {bs.series_number && (
                          <Badge className="text-xs" variant="outline">
                            #{bs.series_number}
                          </Badge>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}

              <Separator />

              {/* Metadata */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm">
                <div>
                  <p className="font-semibold">Created</p>
                  <p className="text-muted-foreground">
                    {formatDate(book.created_at)}
                  </p>
                </div>
                <div>
                  <p className="font-semibold">Updated</p>
                  <p className="text-muted-foreground">
                    {formatDate(book.updated_at)}
                  </p>
                </div>
                <div>
                  <p className="font-semibold">Library</p>
                  <p className="text-muted-foreground">
                    {book.library?.name || `Library ${book.library_id}`}
                  </p>
                </div>
                <div>
                  <p className="font-semibold">File Path</p>
                  <p className="text-muted-foreground break-all">
                    {book.filepath}
                  </p>
                </div>
              </div>

              <Separator />

              {/* Files */}
              <div>
                <h3 className="font-semibold mb-3">
                  Files ({book.files.length})
                </h3>
                <div className="space-y-3">
                  {book.files.map((file) => (
                    <div
                      className="border-l-4 border-l-primary dark:border-l-violet-300 pl-4 py-2 space-y-2"
                      key={file.id}
                    >
                      <div className="flex items-center justify-between gap-4">
                        <div className="flex items-center gap-2 min-w-0 flex-1">
                          <Badge
                            className="uppercase text-xs"
                            variant="secondary"
                          >
                            {file.file_type}
                          </Badge>
                          <span className="font-medium text-sm truncate">
                            {file.filepath.split("/").pop()}
                          </span>
                        </div>

                        <div className="flex items-center gap-3 text-xs text-muted-foreground flex-shrink-0">
                          {file.audiobook_duration && (
                            <span>
                              {formatDuration(file.audiobook_duration)}
                            </span>
                          )}
                          {file.audiobook_bitrate && (
                            <span>
                              {Math.round(file.audiobook_bitrate)} kbps
                            </span>
                          )}
                          <span>{formatFileSize(file.filesize_bytes)}</span>
                          <Button
                            onClick={() => setEditingFile(file)}
                            size="sm"
                            variant="ghost"
                          >
                            <Edit className="h-3 w-3" />
                          </Button>
                        </div>
                      </div>

                      {file.narrators && file.narrators.length > 0 && (
                        <div className="flex items-center gap-2">
                          <span className="text-xs text-muted-foreground">
                            Narrators:
                          </span>
                          <div className="flex items-center gap-1 flex-wrap">
                            {file.narrators.map((narrator, index) => (
                              <Link
                                className="text-xs hover:underline"
                                key={narrator.id}
                                to={`/libraries/${libraryId}/people/${narrator.person_id}`}
                              >
                                {narrator.person?.name ?? "Unknown"}
                                {index < file.narrators!.length - 1 ? "," : ""}
                              </Link>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <BookEditDialog
        book={book}
        onOpenChange={setEditDialogOpen}
        open={editDialogOpen}
      />

      {editingFile && (
        <FileEditDialog
          file={editingFile}
          onOpenChange={(open) => {
            if (!open) setEditingFile(null);
          }}
          open={!!editingFile}
        />
      )}
    </div>
  );
};

export default BookDetail;
