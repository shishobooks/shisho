import { ArrowLeft, Download, Edit, Loader2 } from "lucide-react";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { toast } from "sonner";

import { BookEditDialog } from "@/components/library/BookEditDialog";
import DownloadFormatPopover from "@/components/library/DownloadFormatPopover";
import { FileEditDialog } from "@/components/library/FileEditDialog";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import { useBook } from "@/hooks/queries/books";
import { useLibrary } from "@/hooks/queries/libraries";
import {
  DownloadFormatAsk,
  DownloadFormatKepub,
  FileTypeCBZ,
  FileTypeEPUB,
  type File,
} from "@/types";

// Selects the file that would be used for the cover based on cover_aspect_ratio setting
// This mirrors the backend's selectCoverFile logic
const selectCoverFile = (
  files: File[] | undefined,
  coverAspectRatio: string,
): File | null => {
  if (!files) return null;

  const bookFiles = files.filter(
    (f) =>
      (f.file_type === "epub" || f.file_type === "cbz") && f.cover_image_path,
  );
  const audiobookFiles = files.filter(
    (f) => f.file_type === "m4b" && f.cover_image_path,
  );

  switch (coverAspectRatio) {
    case "audiobook":
    case "audiobook_fallback_book":
      if (audiobookFiles.length > 0) return audiobookFiles[0];
      if (bookFiles.length > 0) return bookFiles[0];
      break;
    default: // "book", "book_fallback_audiobook"
      if (bookFiles.length > 0) return bookFiles[0];
      if (audiobookFiles.length > 0) return audiobookFiles[0];
  }
  return null;
};

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

const getRoleLabel = (role: string | undefined): string | null => {
  if (!role) return null;
  const roleLabels: Record<string, string> = {
    writer: "Writer",
    penciller: "Penciller",
    inker: "Inker",
    colorist: "Colorist",
    letterer: "Letterer",
    cover_artist: "Cover Artist",
    editor: "Editor",
    translator: "Translator",
  };
  return roleLabels[role] || role;
};

interface DownloadError {
  fileId: number;
  message: string;
}

// KePub format is only supported for EPUB and CBZ files
const supportsKepub = (fileType: string): boolean => {
  return fileType === FileTypeEPUB || fileType === FileTypeCBZ;
};

const BookDetail = () => {
  const { id, libraryId } = useParams<{ id: string; libraryId: string }>();
  const bookQuery = useBook(id);
  const libraryQuery = useLibrary(libraryId);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [editingFile, setEditingFile] = useState<File | null>(null);
  const [downloadError, setDownloadError] = useState<DownloadError | null>(
    null,
  );
  const [downloadingFileId, setDownloadingFileId] = useState<number | null>(
    null,
  );

  const handleDownloadWithEndpoint = async (
    fileId: number,
    endpoint: string,
  ) => {
    setDownloadingFileId(fileId);
    try {
      // Use HEAD request to trigger generation and check for errors
      // This avoids loading the entire file into browser memory
      const headResponse = await fetch(endpoint, {
        method: "HEAD",
      });

      if (!headResponse.ok) {
        // HEAD failed - make a GET request to get the error message (small JSON response)
        const errorResponse = await fetch(endpoint);
        const contentType = errorResponse.headers.get("content-type");
        if (contentType && contentType.includes("application/json")) {
          const error = await errorResponse.json();
          setDownloadError({
            fileId,
            message: error.message || "Failed to generate file",
          });
        } else {
          setDownloadError({
            fileId,
            message: "Failed to download file",
          });
        }
        return;
      }

      // HEAD succeeded - file is ready, trigger streaming download
      window.location.href = endpoint;
      toast.success("Download started");
    } catch (error) {
      console.error("Download error:", error);
      toast.error("Failed to download file");
    } finally {
      setDownloadingFileId(null);
    }
  };

  const handleDownload = async (fileId: number, fileType: string) => {
    const preference = libraryQuery.data?.download_format_preference;

    // For kepub preference with supported files, use kepub endpoint
    if (preference === DownloadFormatKepub && supportsKepub(fileType)) {
      await handleDownloadWithEndpoint(
        fileId,
        `/api/books/files/${fileId}/download/kepub`,
      );
    } else {
      // Original format for unsupported files or "original" preference
      await handleDownloadWithEndpoint(
        fileId,
        `/api/books/files/${fileId}/download`,
      );
    }
  };

  const handleDownloadKepub = async (fileId: number) => {
    await handleDownloadWithEndpoint(
      fileId,
      `/api/books/files/${fileId}/download/kepub`,
    );
  };

  const handleDownloadOriginal = (fileId: number) => {
    // Direct download of original file - this won't show any error since it's a simple file serve
    window.location.href = `/api/books/files/${fileId}/download/original`;
    setDownloadError(null);
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

  // Determine which file's cover is being displayed based on library's cover_aspect_ratio setting
  const libraryCoverAspectRatio =
    libraryQuery.data?.cover_aspect_ratio ?? "book";
  const coverFile = selectCoverFile(book.files, libraryCoverAspectRatio);
  const isAudiobookCover = coverFile?.file_type === "m4b";
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
            <div className={`${coverAspectRatio} w-full`}>
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
              {book.sort_title && book.sort_title !== book.title && (
                <p className="text-sm text-muted-foreground italic">
                  Sort title: {book.sort_title}
                </p>
              )}
              {book.subtitle && (
                <p className="text-lg text-muted-foreground">{book.subtitle}</p>
              )}
            </div>

            <div className="space-y-6">
              {/* Authors */}
              {book.authors &&
                book.authors.length > 0 &&
                (() => {
                  const hasCBZFiles = book.files?.some(
                    (f) => f.file_type === FileTypeCBZ,
                  );
                  return (
                    <div>
                      <h3 className="font-semibold mb-2">Authors</h3>
                      <div className="flex flex-wrap gap-2">
                        {book.authors.map((author) => {
                          const roleLabel = getRoleLabel(author.role);
                          return (
                            <Link
                              key={author.id}
                              to={`/libraries/${libraryId}/people/${author.person_id}`}
                            >
                              <Badge
                                className="cursor-pointer hover:bg-secondary/80"
                                variant="secondary"
                              >
                                {author.person?.name ?? "Unknown"}
                                {hasCBZFiles && roleLabel && (
                                  <span className="text-muted-foreground ml-1">
                                    ({roleLabel})
                                  </span>
                                )}
                              </Badge>
                            </Link>
                          );
                        })}
                      </div>
                    </div>
                  );
                })()}

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

              {/* Genres */}
              {book.book_genres && book.book_genres.length > 0 && (
                <div>
                  <h3 className="font-semibold mb-2">Genres</h3>
                  <div className="flex flex-wrap gap-2">
                    {book.book_genres.map((bg) => (
                      <Link
                        key={bg.id}
                        to={`/libraries/${libraryId}?genre_ids=${bg.genre_id}`}
                      >
                        <Badge
                          className="cursor-pointer hover:bg-secondary/80"
                          variant="secondary"
                        >
                          {bg.genre?.name ?? "Unknown"}
                        </Badge>
                      </Link>
                    ))}
                  </div>
                </div>
              )}

              {/* Tags */}
              {book.book_tags && book.book_tags.length > 0 && (
                <div>
                  <h3 className="font-semibold mb-2">Tags</h3>
                  <div className="flex flex-wrap gap-2">
                    {book.book_tags.map((bt) => (
                      <Link
                        key={bt.id}
                        to={`/libraries/${libraryId}?tag_ids=${bt.tag_id}`}
                      >
                        <Badge
                          className="cursor-pointer hover:bg-secondary/80"
                          variant="secondary"
                        >
                          {bt.tag?.name ?? "Unknown"}
                        </Badge>
                      </Link>
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
                          {file.audiobook_duration_seconds && (
                            <span>
                              {formatDuration(file.audiobook_duration_seconds)}
                            </span>
                          )}
                          {file.audiobook_bitrate_bps && (
                            <span>
                              {Math.round(file.audiobook_bitrate_bps / 1000)}{" "}
                              kbps
                            </span>
                          )}
                          <span>{formatFileSize(file.filesize_bytes)}</span>
                          {/* Show format popover for "ask" preference on EPUB/CBZ files */}
                          {libraryQuery.data?.download_format_preference ===
                            DownloadFormatAsk &&
                          supportsKepub(file.file_type) ? (
                            <DownloadFormatPopover
                              disabled={downloadingFileId === file.id}
                              isLoading={downloadingFileId === file.id}
                              onDownloadKepub={() =>
                                handleDownloadKepub(file.id)
                              }
                              onDownloadOriginal={() =>
                                handleDownloadWithEndpoint(
                                  file.id,
                                  `/api/books/files/${file.id}/download`,
                                )
                              }
                            />
                          ) : (
                            <Button
                              disabled={downloadingFileId === file.id}
                              onClick={() =>
                                handleDownload(file.id, file.file_type)
                              }
                              size="sm"
                              title="Download"
                              variant="ghost"
                            >
                              {downloadingFileId === file.id ? (
                                <Loader2 className="h-3 w-3 animate-spin" />
                              ) : (
                                <Download className="h-3 w-3" />
                              )}
                            </Button>
                          )}
                          <Button
                            onClick={() => setEditingFile(file)}
                            size="sm"
                            title="Edit"
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

      {/* Download Error Dialog */}
      <Dialog
        onOpenChange={(open) => {
          if (!open) setDownloadError(null);
        }}
        open={!!downloadError}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Download Failed</DialogTitle>
            <DialogDescription>{downloadError?.message}</DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button onClick={() => setDownloadError(null)} variant="outline">
              Cancel
            </Button>
            {downloadError && (
              <Button
                onClick={() => handleDownloadOriginal(downloadError.fileId)}
              >
                Download Original
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default BookDetail;
