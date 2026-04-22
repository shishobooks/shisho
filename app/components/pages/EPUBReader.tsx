import { AlertCircle, ArrowLeft, Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { useEpubBlob } from "@/hooks/queries/epub";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { File } from "@/types";

interface EPUBReaderProps {
  file: File;
  libraryId: string;
  bookTitle?: string;
}

export default function EPUBReader({
  file,
  libraryId,
  bookTitle,
}: EPUBReaderProps) {
  usePageTitle(bookTitle ? `Reading: ${bookTitle}` : "Reader");

  const {
    data: blob,
    isLoading,
    isError,
    error,
    refetch,
  } = useEpubBlob(file.id);

  const [showExtendedHint, setShowExtendedHint] = useState(false);
  useEffect(() => {
    if (!isLoading) {
      setShowExtendedHint(false);
      return;
    }
    const timer = setTimeout(() => setShowExtendedHint(true), 10_000);
    return () => clearTimeout(timer);
  }, [isLoading]);

  const backHref = `/libraries/${libraryId}/books/${file.book_id}`;

  if (isError) {
    return (
      <div className="fixed inset-0 bg-background flex flex-col items-center justify-center gap-4 p-4 text-center">
        <AlertCircle className="h-8 w-8 text-destructive" />
        <div>
          <p className="font-medium">We couldn't load this book.</p>
          <p className="text-sm text-muted-foreground mt-1">
            {error?.message ?? "Unknown error"}
          </p>
        </div>
        <div className="flex gap-2">
          <Button onClick={() => refetch()} variant="default">
            Retry
          </Button>
          <Button asChild variant="outline">
            <Link to={backHref}>Back</Link>
          </Button>
        </div>
      </div>
    );
  }

  if (isLoading || !blob) {
    return (
      <div className="fixed inset-0 bg-background flex flex-col items-center justify-center gap-3">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        <p className="text-sm text-muted-foreground">Preparing book…</p>
        {showExtendedHint && (
          <p className="text-xs text-muted-foreground">
            This may take a moment for large books.
          </p>
        )}
      </div>
    );
  }

  // Reader chrome rendered in Task 6.
  return (
    <div className="fixed inset-0 bg-background flex flex-col">
      <header className="flex items-center justify-between px-4 py-2 border-b">
        <Link
          className="flex items-center gap-2 text-muted-foreground hover:text-foreground"
          to={backHref}
        >
          <ArrowLeft className="h-4 w-4" />
          <span className="text-sm">Back</span>
        </Link>
      </header>
      <main className="flex-1 bg-background" />
      <footer className="border-t px-4 py-2 text-xs text-muted-foreground">
        Loaded {(blob.size / 1024 / 1024).toFixed(1)} MB
      </footer>
    </div>
  );
}
