import {
  ArrowLeft,
  ChevronDown,
  ChevronRight,
  Download,
  Edit,
  Loader2,
  X,
} from "lucide-react";
import React, { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { toast } from "sonner";

import { BookEditDialog } from "@/components/library/BookEditDialog";
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
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

// Determines which file type would provide the cover based on library preference.
// This mirrors the backend's selectCoverFile priority logic but doesn't require cover_image_path.
// Used for placeholder variant selection when there's no cover image.
const getCoverFileType = (
  files: File[] | undefined,
  coverAspectRatio: string,
): "book" | "audiobook" => {
  if (!files || files.length === 0) return "book";

  const hasBookFiles = files.some(
    (f) => f.file_type === "epub" || f.file_type === "cbz",
  );
  const hasAudiobookFiles = files.some((f) => f.file_type === "m4b");

  switch (coverAspectRatio) {
    case "audiobook":
    case "audiobook_fallback_book":
      if (hasAudiobookFiles) return "audiobook";
      if (hasBookFiles) return "book";
      break;
    default: // "book", "book_fallback_audiobook"
      if (hasBookFiles) return "book";
      if (hasAudiobookFiles) return "audiobook";
  }
  return "book";
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

// Helper to extract filename from filepath
const getFilename = (filepath: string): string => {
  return filepath.split("/").pop() || filepath;
};

function formatIdentifierType(type: string): string {
  switch (type) {
    case "isbn_10":
      return "ISBN-10";
    case "isbn_13":
      return "ISBN-13";
    case "asin":
      return "ASIN";
    case "uuid":
      return "UUID";
    case "goodreads":
      return "Goodreads";
    case "google":
      return "Google";
    case "other":
      return "Other";
    default:
      return type;
  }
}

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

interface FileRowProps {
  file: File;
  libraryId: string;
  libraryDownloadPreference: string | undefined;
  isExpanded: boolean;
  hasExpandableMetadata: boolean;
  onToggleExpand: () => void;
  isDownloading: boolean;
  onDownload: () => void;
  onDownloadKepub: () => void;
  onDownloadOriginal: () => void;
  onDownloadWithEndpoint: (endpoint: string) => void;
  onCancelDownload: () => void;
  onEdit: () => void;
  isSupplement?: boolean;
}

const FileRow = ({
  file,
  libraryId,
  libraryDownloadPreference,
  isExpanded,
  hasExpandableMetadata,
  onToggleExpand,
  isDownloading,
  onDownload,
  onDownloadKepub,
  onDownloadOriginal,
  onDownloadWithEndpoint,
  onCancelDownload,
  onEdit,
  isSupplement = false,
}: FileRowProps) => {
  const showChevron = hasExpandableMetadata && !isSupplement;

  return (
    <div className="py-2 space-y-1">
      {/* Primary row */}
      <div className="flex items-center gap-2">
        {/* Clickable area for expand/collapse (chevron, badge, name) */}
        <div
          aria-expanded={showChevron ? isExpanded : undefined}
          className={`flex items-center gap-2 min-w-0 flex-1 rounded-md -ml-1 pl-1 ${
            showChevron ? "cursor-pointer hover:bg-muted/50" : ""
          }`}
          onClick={showChevron ? onToggleExpand : undefined}
          onKeyDown={
            showChevron
              ? (e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    onToggleExpand();
                  }
                }
              : undefined
          }
          role={showChevron ? "button" : undefined}
          tabIndex={showChevron ? 0 : undefined}
        >
          {/* Chevron indicator */}
          {showChevron ? (
            <div className="p-0.5">
              {isExpanded ? (
                <ChevronDown className="h-4 w-4 text-muted-foreground" />
              ) : (
                <ChevronRight className="h-4 w-4 text-muted-foreground" />
              )}
            </div>
          ) : (
            <div className="w-5" /> // Spacer for alignment when no chevron
          )}

          {/* File type badge */}
          <Badge
            className="uppercase text-xs flex-shrink-0"
            variant={isSupplement ? "outline" : "secondary"}
          >
            {file.file_type}
          </Badge>

          {/* Name */}
          <div className="flex flex-col min-w-0 flex-1">
            <span
              className={`truncate ${isSupplement ? "text-sm" : "text-sm font-medium"}`}
              title={file.name || getFilename(file.filepath)}
            >
              {file.name || getFilename(file.filepath)}
            </span>
          </div>
        </div>

        {/* Stats and actions */}
        <div className="flex items-center gap-3 text-xs text-muted-foreground flex-shrink-0">
          {/* M4B stats */}
          {file.audiobook_duration_seconds && (
            <span>{formatDuration(file.audiobook_duration_seconds)}</span>
          )}
          {file.audiobook_bitrate_bps && (
            <span>{Math.round(file.audiobook_bitrate_bps / 1000)} kbps</span>
          )}
          {/* CBZ stats */}
          {file.page_count && <span>{file.page_count} pages</span>}
          {/* File size - always shown */}
          <span>{formatFileSize(file.filesize_bytes)}</span>

          {/* Download button/popover */}
          {isSupplement ? (
            <Button
              onClick={onDownloadOriginal}
              size="sm"
              title="Download"
              variant="ghost"
            >
              <Download className="h-3 w-3" />
            </Button>
          ) : libraryDownloadPreference === DownloadFormatAsk &&
            supportsKepub(file.file_type) ? (
            <DownloadFormatPopover
              disabled={isDownloading}
              isLoading={isDownloading}
              onCancel={onCancelDownload}
              onDownloadKepub={onDownloadKepub}
              onDownloadOriginal={() =>
                onDownloadWithEndpoint(`/api/books/files/${file.id}/download`)
              }
            />
          ) : isDownloading ? (
            <div className="flex items-center gap-1">
              <Loader2 className="h-3 w-3 animate-spin" />
              <Button
                className="h-6 w-6 p-0"
                onClick={onCancelDownload}
                size="sm"
                title="Cancel download"
                variant="ghost"
              >
                <X className="h-3 w-3" />
              </Button>
            </div>
          ) : (
            <Button
              onClick={onDownload}
              size="sm"
              title="Download"
              variant="ghost"
            >
              <Download className="h-3 w-3" />
            </Button>
          )}

          {/* Read button - CBZ only */}
          {file.file_type === FileTypeCBZ && (
            <Link
              to={`/libraries/${libraryId}/books/${file.book_id}/files/${file.id}/read`}
            >
              <Button size="sm" title="Read" variant="ghost">
                Read
              </Button>
            </Link>
          )}

          {/* Edit button */}
          <Button onClick={onEdit} size="sm" title="Edit" variant="ghost">
            <Edit className="h-3 w-3" />
          </Button>
        </div>
      </div>

      {/* Filename row - only show when name differs from filename */}
      {file.name && (
        <div className="ml-5 pl-2">
          <span
            className="text-xs text-muted-foreground truncate block"
            title={file.filepath}
          >
            {getFilename(file.filepath)}
          </span>
        </div>
      )}

      {/* Narrators row - M4B only, always visible when present */}
      {file.narrators && file.narrators.length > 0 && (
        <div className="ml-5 pl-2 flex items-center gap-1 flex-wrap">
          <span className="text-xs text-muted-foreground">Narrated by</span>
          {file.narrators.map((narrator, index) => (
            <span className="text-xs" key={narrator.id}>
              <Link
                className="hover:underline"
                to={`/libraries/${libraryId}/people/${narrator.person_id}`}
              >
                {narrator.person?.name ?? "Unknown"}
              </Link>
              {index < file.narrators!.length - 1 ? "," : ""}
            </span>
          ))}
        </div>
      )}

      {/* Expandable details section */}
      {isExpanded && hasExpandableMetadata && (
        <div className="ml-5 pl-2 mt-2 bg-muted/50 rounded-md p-3 text-xs space-y-2">
          {/* Publisher, Imprint, Released, URL */}
          <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
            {file.publisher && (
              <>
                <span className="text-muted-foreground">Publisher</span>
                <span>{file.publisher.name}</span>
              </>
            )}
            {file.imprint && (
              <>
                <span className="text-muted-foreground">Imprint</span>
                <span>{file.imprint.name}</span>
              </>
            )}
            {file.release_date && (
              <>
                <span className="text-muted-foreground">Released</span>
                <span>{formatDate(file.release_date)}</span>
              </>
            )}
            {file.url && (
              <>
                <span className="text-muted-foreground">URL</span>
                <a
                  className="text-primary hover:underline truncate"
                  href={file.url}
                  rel="noopener noreferrer"
                  target="_blank"
                  title={file.url}
                >
                  {file.url.length > 60
                    ? file.url.substring(0, 60) + "..."
                    : file.url}
                </a>
              </>
            )}
          </div>

          {/* Identifiers */}
          {file.identifiers && file.identifiers.length > 0 && (
            <div className="pt-2 border-t border-border/50">
              <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
                {file.identifiers.map((id, idx) => (
                  <React.Fragment key={idx}>
                    <span className="text-muted-foreground">
                      {formatIdentifierType(id.type)}
                    </span>
                    <span className="font-mono select-all">{id.value}</span>
                  </React.Fragment>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
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
  const [coverError, setCoverError] = useState(false);
  const [expandedFileIds, setExpandedFileIds] = useState<Set<number>>(
    new Set(),
  );
  const downloadAbortController = useRef<AbortController | null>(null);

  const toggleFileExpanded = (fileId: number) => {
    setExpandedFileIds((prev) => {
      const next = new Set(prev);
      if (next.has(fileId)) {
        next.delete(fileId);
      } else {
        next.add(fileId);
      }
      return next;
    });
  };

  const hasExpandableMetadata = (file: File): boolean => {
    return !!(
      file.publisher ||
      file.imprint ||
      file.release_date ||
      file.url ||
      (file.identifiers && file.identifiers.length > 0)
    );
  };

  useEffect(() => {
    setCoverError(false);
  }, [bookQuery.data?.id]);

  const handleDownloadWithEndpoint = async (
    fileId: number,
    endpoint: string,
  ) => {
    setDownloadError(null);
    setDownloadingFileId(fileId);

    // Create abort controller for this download
    const abortController = new AbortController();
    downloadAbortController.current = abortController;

    try {
      // Use HEAD request to trigger generation and check for errors
      // This avoids loading the entire file into browser memory
      const headResponse = await fetch(endpoint, {
        method: "HEAD",
        signal: abortController.signal,
      });

      if (!headResponse.ok) {
        // HEAD failed - make a GET request to get the error message (small JSON response)
        const errorResponse = await fetch(endpoint, {
          signal: abortController.signal,
        });
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
      // Don't show error dialog for user-initiated cancellation
      if (error instanceof DOMException && error.name === "AbortError") {
        return;
      }
      console.error("Download error:", error);
      toast.error("Failed to download file");
    } finally {
      downloadAbortController.current = null;
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

  const handleCancelDownload = () => {
    downloadAbortController.current?.abort();
    downloadAbortController.current = null;
    setDownloadingFileId(null);
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

  // Separate main files and supplements
  const mainFiles =
    book.files?.filter((f) => f.file_role !== "supplement") ?? [];
  const supplements =
    book.files?.filter((f) => f.file_role === "supplement") ?? [];

  // Determine which file type would provide the cover based on library's cover_aspect_ratio setting
  // This is used to determine the native aspect ratio (audiobook = square, book = 2:3)
  const libraryCoverAspectRatio =
    libraryQuery.data?.cover_aspect_ratio ?? "book";
  const coverFileType = getCoverFileType(book.files, libraryCoverAspectRatio);
  const isAudiobook = coverFileType === "audiobook";
  const coverAspectRatio = isAudiobook ? "aspect-square" : "aspect-[2/3]";

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
              {!coverError ? (
                <img
                  alt={`${book.title} Cover`}
                  className="w-full h-full object-cover rounded-md border border-border"
                  onError={() => setCoverError(true)}
                  src={`/api/books/${book.id}/cover?t=${coverCacheBuster}`}
                />
              ) : (
                <CoverPlaceholder
                  className={`rounded-md border border-border ${coverAspectRatio}`}
                  variant={coverFileType}
                />
              )}
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
              {book.description && (
                <p className="text-sm text-muted-foreground mt-3 whitespace-pre-wrap">
                  {book.description}
                </p>
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
                  Files ({mainFiles.length})
                </h3>
                <div className="space-y-2">
                  {mainFiles.map((file) => (
                    <FileRow
                      file={file}
                      hasExpandableMetadata={hasExpandableMetadata(file)}
                      isDownloading={downloadingFileId === file.id}
                      isExpanded={expandedFileIds.has(file.id)}
                      key={file.id}
                      libraryDownloadPreference={
                        libraryQuery.data?.download_format_preference
                      }
                      libraryId={libraryId!}
                      onCancelDownload={handleCancelDownload}
                      onDownload={() => handleDownload(file.id, file.file_type)}
                      onDownloadKepub={() => handleDownloadKepub(file.id)}
                      onDownloadOriginal={() => handleDownloadOriginal(file.id)}
                      onDownloadWithEndpoint={(endpoint) =>
                        handleDownloadWithEndpoint(file.id, endpoint)
                      }
                      onEdit={() => setEditingFile(file)}
                      onToggleExpand={() => toggleFileExpanded(file.id)}
                    />
                  ))}
                </div>
              </div>

              {/* Supplements */}
              {supplements.length > 0 && (
                <>
                  <Separator />
                  <div>
                    <h3 className="font-semibold mb-3">
                      Supplements ({supplements.length})
                    </h3>
                    <div className="space-y-2">
                      {supplements.map((file) => (
                        <FileRow
                          file={file}
                          hasExpandableMetadata={hasExpandableMetadata(file)}
                          isDownloading={downloadingFileId === file.id}
                          isExpanded={expandedFileIds.has(file.id)}
                          isSupplement
                          key={file.id}
                          libraryDownloadPreference={
                            libraryQuery.data?.download_format_preference
                          }
                          libraryId={libraryId!}
                          onCancelDownload={handleCancelDownload}
                          onDownload={() =>
                            handleDownload(file.id, file.file_type)
                          }
                          onDownloadKepub={() => handleDownloadKepub(file.id)}
                          onDownloadOriginal={() =>
                            handleDownloadOriginal(file.id)
                          }
                          onDownloadWithEndpoint={(endpoint) =>
                            handleDownloadWithEndpoint(file.id, endpoint)
                          }
                          onEdit={() => setEditingFile(file)}
                          onToggleExpand={() => toggleFileExpanded(file.id)}
                        />
                      ))}
                    </div>
                  </div>
                </>
              )}
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
