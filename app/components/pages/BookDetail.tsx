import { ArrowLeft, Info } from "lucide-react";
import { Link, useParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useBook } from "@/hooks/queries/books";

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
  const { id } = useParams<{ id: string }>();
  const bookQuery = useBook(id);

  if (bookQuery.isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full p-8 m-auto">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (!bookQuery.isSuccess || !bookQuery.data) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full p-8 m-auto">
          <div className="text-center">
            <h1 className="text-2xl font-bold mb-4">Book Not Found</h1>
            <p className="text-muted-foreground mb-6">
              The book you're looking for doesn't exist or may have been
              removed.
            </p>
            <Button asChild>
              <Link to="/">
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

  return (
    <TooltipProvider>
      <div>
        <TopNav />
        <div className="max-w-7xl w-full p-8 m-auto">
          <div className="mb-6">
            <Button asChild variant="ghost">
              <Link to="/">
                <ArrowLeft className="mr-2 h-4 w-4" />
                Back to Books
              </Link>
            </Button>
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
            {/* Book Cover and Basic Info */}
            <div className="lg:col-span-1">
              <Card>
                <CardContent className="px-6">
                  <div className={`${coverAspectRatio} w-full`}>
                    <img
                      alt={`${book.title} Cover`}
                      className="w-full h-full object-cover rounded-md border"
                      onError={(e) => {
                        (e.target as HTMLImageElement).style.display = "none";
                        (
                          e.target as HTMLImageElement
                        ).nextElementSibling!.textContent =
                          "No cover available";
                      }}
                      src={`/api/books/${book.id}/cover`}
                    />
                    <div className="hidden text-center text-muted-foreground"></div>
                  </div>
                </CardContent>
              </Card>
            </div>

            {/* Book Details */}
            <div className="lg:col-span-2">
              <Card>
                <CardHeader>
                  <div className="flex items-start gap-3">
                    <CardTitle className="text-3xl flex-1">
                      {book.title}
                    </CardTitle>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Info className="h-5 w-5 text-muted-foreground cursor-help flex-shrink-0 mt-1" />
                      </TooltipTrigger>
                      <TooltipContent>
                        <p>
                          Title source: {book.title_source.replace("_", " ")}
                        </p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  {book.subtitle && (
                    <CardDescription className="text-lg">
                      {book.subtitle}
                    </CardDescription>
                  )}
                </CardHeader>

                <CardContent className="space-y-6">
                  {/* Authors */}
                  {book.authors && book.authors.length > 0 && (
                    <div>
                      <div className="flex items-center gap-2 mb-2">
                        <h3 className="font-semibold">Authors</h3>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Info className="h-3 w-3 text-muted-foreground cursor-help" />
                          </TooltipTrigger>
                          <TooltipContent>
                            <p>
                              Authors source:{" "}
                              {book.author_source.replace("_", " ")}
                            </p>
                          </TooltipContent>
                        </Tooltip>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        {book.authors.map((author) => (
                          <Badge key={author.id} variant="secondary">
                            {author.name}
                          </Badge>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Series */}
                  {book.series && (
                    <div>
                      <h3 className="font-semibold mb-2">Series</h3>
                      <p className="text-sm text-muted-foreground">
                        {book.series}
                        {book.series_number && ` #${book.series_number}`}
                      </p>
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
                    <h3 className="font-semibold mb-4">
                      Files ({book.files.length})
                    </h3>
                    <div className="space-y-4">
                      {book.files.map((file) => (
                        <Card
                          className="border-l-4 border-l-blue-500"
                          key={file.id}
                        >
                          <CardContent className="p-4">
                            <div className="flex items-start justify-between mb-2">
                              <div className="flex items-center gap-2">
                                <Badge className="uppercase" variant="subtle">
                                  {file.file_type}
                                </Badge>
                                <span className="font-medium">
                                  {file.filepath.split("/").pop()}
                                </span>
                              </div>
                              <span className="text-sm text-muted-foreground">
                                {formatFileSize(file.filesize_bytes)}
                              </span>
                            </div>

                            <div className="text-sm text-muted-foreground space-y-1">
                              <p className="break-all">{file.filepath}</p>

                              {file.audiobook_duration && (
                                <p>
                                  Duration:{" "}
                                  {formatDuration(file.audiobook_duration)}
                                </p>
                              )}

                              {file.audiobook_bitrate && (
                                <p>
                                  Bitrate: {Math.round(file.audiobook_bitrate)}{" "}
                                  kbps
                                </p>
                              )}

                              {file.narrators && file.narrators.length > 0 && (
                                <div>
                                  <span>Narrators: </span>
                                  {file.narrators.map((narrator, index) => (
                                    <span key={narrator.id}>
                                      {narrator.name}
                                      {index < file.narrators!.length - 1
                                        ? ", "
                                        : ""}
                                    </span>
                                  ))}
                                  {file.narrator_source && (
                                    <Badge className="ml-2" variant="outline">
                                      {file.narrator_source.replace("_", " ")}
                                    </Badge>
                                  )}
                                </div>
                              )}

                              <div className="flex items-center gap-2 text-xs">
                                <span>
                                  Created: {formatDate(file.created_at)}
                                </span>
                                <span>•</span>
                                <span>
                                  Updated: {formatDate(file.updated_at)}
                                </span>
                              </div>
                            </div>
                          </CardContent>
                        </Card>
                      ))}
                    </div>
                  </div>
                </CardContent>
              </Card>
            </div>
          </div>
        </div>
      </div>
    </TooltipProvider>
  );
};

export default BookDetail;
