import { Share2 } from "lucide-react";
import { useParams } from "react-router-dom";

import BookDetailBody, {
  type ShareLinkContext,
} from "@/components/library/BookDetailBody";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import Logo from "@/components/library/Logo";
import { Button } from "@/components/ui/button";
import { Toaster } from "@/components/ui/sonner";
import { useSharedBook } from "@/hooks/queries/sharing";
import { usePageTitle } from "@/hooks/usePageTitle";
import { ShishoAPIError } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import type { SharedBookResponse } from "@/types";
import { formatDateTime } from "@/utils/format";

// Builds the public endpoints the body uses in place of the authenticated
// ones. The payload has its cover filenames blanked, so covers are keyed off
// the book's cover_cache_key and requested for every main file; a missing
// file cover 404s and the thumbnail falls back to the placeholder.
const shareLinkContext = (
  token: string,
  shared: SharedBookResponse,
): ShareLinkContext => {
  const base = `/api/share/${encodeURIComponent(token)}`;
  return {
    downloadUrl: (file) => `${base}/files/${file.id}/download`,
    bookCoverUrl: (book) =>
      book.cover_cache_key ? `${base}/cover?v=${book.cover_cache_key}` : null,
    fileCoverUrl: (file) =>
      file.file_role === "supplement"
        ? null
        : `${base}/files/${file.id}/cover?v=${encodeURIComponent(file.updated_at)}`,
    coverAspectRatio: shared.cover_aspect_ratio,
  };
};

// The one page for every link that cannot be used: unknown, mistyped,
// expired, revoked, or with sharing turned off (every 404 from the server).
// It never says which.
export const ShareUnavailable = () => (
  <div className="text-center py-16">
    <h1 className="text-2xl font-semibold mb-2">
      This link is no longer available
    </h1>
    <p className="text-muted-foreground">
      Ask the person who shared it to send you a new one.
    </p>
  </div>
);

// A full-width strip under the header saying who shared the book and when
// the link expires, styled like the Demo Mode banner.
const ShareNotice = ({ shared }: { shared: SharedBookResponse }) => (
  <aside
    aria-label="Share details"
    className="border-y border-primary/20 bg-primary/10 text-sm"
  >
    <div className="mx-auto flex max-w-7xl items-center gap-3 px-4 py-2.5 md:px-6">
      <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-primary/15 text-primary">
        <Share2 aria-hidden="true" className="size-3.5" />
      </span>
      <p className="min-w-0">
        <span className="font-semibold break-words">{shared.shared_by}</span>{" "}
        shared this book with you.
        {shared.expires_at && (
          // Muted foreground falls below AA contrast on the tint, so the
          // secondary sentence uses softened foreground instead.
          <span className="text-foreground/80">
            {" "}
            This link expires {formatDateTime(shared.expires_at)}.
          </span>
        )}
      </p>
    </div>
  </aside>
);

// The recipient page for a Share Link. It sits outside the protected route
// tree: no login redirect, no navigation, no sidebar, and no links into the
// app. The book body renders in Share Link context.
const SharedBook = () => {
  const { token } = useParams<{ token: string }>();
  const sharedQuery = useSharedBook(token);
  const shared = sharedQuery.data;

  const showNotice = Boolean(shared && token);

  usePageTitle(shared?.title ?? "Shared Book");

  let content;
  if (sharedQuery.isLoading) {
    content = <LoadingSpinner />;
  } else if (
    !token ||
    (sharedQuery.error instanceof ShishoAPIError &&
      sharedQuery.error.status === 404)
  ) {
    content = <ShareUnavailable />;
  } else if (sharedQuery.isError || !shared) {
    // The server failed or could not be reached; the link may be fine.
    content = (
      <div className="text-center py-16">
        <h1 className="text-2xl font-semibold mb-2">
          Could not load this link
        </h1>
        <p className="text-muted-foreground mb-4">
          The server did not respond. Check your connection and try again.
        </p>
        <Button
          disabled={sharedQuery.isFetching}
          onClick={() => sharedQuery.refetch()}
          variant="outline"
        >
          Try again
        </Button>
      </div>
    );
  } else {
    content = (
      <BookDetailBody
        book={shared}
        shareLink={shareLinkContext(token, shared)}
      />
    );
  }

  return (
    <div className="min-h-screen bg-background font-sans">
      {/* The notice strip draws its own top border in the same tint as its
          bottom one, so the header drops its neutral border above it. */}
      <header className={cn(!showNotice && "border-b border-border")}>
        <div className="max-w-7xl mx-auto px-4 md:px-6 h-14 flex items-center">
          <Logo />
        </div>
      </header>
      {shared && token && <ShareNotice shared={shared} />}
      <main className="max-w-7xl mx-auto px-4 py-4 md:px-6 md:py-8">
        {content}
      </main>
      <Toaster richColors />
    </div>
  );
};

export default SharedBook;
