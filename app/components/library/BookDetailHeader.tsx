import { useEffect, useLayoutEffect, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";

import CoverImage from "@/components/library/CoverImage";
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import FadedText from "@/components/library/FadedText";
import LabelGrid, { type LabelGridRow } from "@/components/library/LabelGrid";
import TextLink from "@/components/library/TextLink";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/libraries/utils";
import type { Book } from "@/types";
import { getAuthorRoleLabel } from "@/utils/authorRoles";
import { isCoverLoaded, markCoverLoaded } from "@/utils/coverCache";
import { hasAnyCBZFile } from "@/utils/hasAnyCBZFile";
import { seriesNumberLabel } from "@/utils/seriesNumber";
import { splitTagNamespace } from "@/utils/tagNamespaces";

const Pill = ({ to, children }: { to: string | null; children: ReactNode }) => {
  const badge = (
    <Badge className={cn(to && "hover:bg-secondary/70")} variant="secondary">
      {children}
    </Badge>
  );
  return to ? <Link to={to}>{badge}</Link> : badge;
};

/** One entry per person, with every role they hold on the book. */
const creditedPeople = (book: Book) => {
  const people = new Map<
    number,
    { personId: number; name: string; roles: string[]; linked: boolean }
  >();
  for (const author of book.authors ?? []) {
    const entry = people.get(author.person_id) ?? {
      personId: author.person_id,
      name: author.person?.name ?? "Unknown",
      roles: [],
      linked: Boolean(author.person),
    };
    const role = getAuthorRoleLabel(author.role);
    if (role && !entry.roles.includes(role)) entry.roles.push(role);
    people.set(author.person_id, entry);
  }
  return [...people.values()];
};

interface BookDetailHeaderProps {
  book: Book;
  /** The book cover, or null when it has none. */
  coverUrl: string | null;
  /** The shape the cover box takes, from the library's cover preference. */
  coverVariant: "book" | "audiobook";
  /** Drawn over the cover's bottom-left corner, as the gallery does. */
  coverBadge?: ReactNode;
  /** The book's action buttons. */
  actions?: ReactNode;
  /** The signed-in user's Reading Status and Rating, under the actions. */
  reading?: ReactNode;
  /** Beside the header from 1280px; the caller places it otherwise. */
  sidePanel?: ReactNode;
  /** Share Link context: every name is plain text. */
  isShareLink: boolean;
  linkPeople: boolean;
  linkSeries: boolean;
  /** The library's tag namespaces, each listed as its own row. */
  tagNamespaces: readonly string[];
}

/**
 * The top of the book page: the cover, then the title, who wrote it, the
 * actions, the reader's status and rating, the series, genres, and tags, and the description, which fades
 * out when long. Raw details (paths, dates, sort title) live in the Book
 * details dialog, off the page.
 */
const BookDetailHeader = ({
  book,
  coverUrl,
  coverVariant,
  coverBadge,
  actions,
  reading,
  sidePanel,
  isShareLink,
  linkPeople,
  linkSeries,
  tagNamespaces,
}: BookDetailHeaderProps) => {
  const [coverLoaded, setCoverLoaded] = useState(false);
  const [coverError, setCoverError] = useState(false);

  // Reset the error flag whenever the URL changes, either because we
  // navigated to a different book or because a rescan bumped the cover's
  // cache key. If it reset only on the book id, a previously missing cover
  // would stay unmounted after a successful rescan until a refresh.
  useEffect(() => {
    setCoverError(false);
  }, [coverUrl]);

  // Check the cache before paint to avoid a placeholder flash.
  useLayoutEffect(() => {
    if (coverUrl && isCoverLoaded(coverUrl)) setCoverLoaded(true);
  }, [coverUrl]);

  const comic = hasAnyCBZFile(book);
  const people = creditedPeople(book);
  const libraryId = book.library_id;

  const rows: LabelGridRow[] = [];
  if (book.book_series?.length) {
    rows.push({
      label: "Series",
      value: (
        <span className="flex flex-wrap gap-x-1">
          {book.book_series.map((bs, index) => {
            const number = seriesNumberLabel(
              bs.series_number,
              bs.series_number_end,
              bs.series_number_unit,
              comic ? "cbz" : null,
            );
            return (
              <span key={bs.id}>
                <TextLink
                  to={
                    linkSeries
                      ? `/libraries/${libraryId}/series/${bs.series_id}`
                      : null
                  }
                >
                  {bs.series?.name ?? "Unknown Series"}
                  {number && ` ${number}`}
                </TextLink>
                {index < book.book_series!.length - 1 && ","}
              </span>
            );
          })}
        </span>
      ),
    });
  }
  if (book.book_genres?.length) {
    rows.push({
      label: "Genres",
      value: (
        <span className="flex flex-wrap gap-1.5">
          {book.book_genres.map((bg) => (
            <Pill
              key={bg.id}
              to={
                isShareLink
                  ? null
                  : `/libraries/${libraryId}?genre_ids=${bg.genre_id}`
              }
            >
              {bg.genre?.name ?? "Unknown"}
            </Pill>
          ))}
        </span>
      ),
    });
  }
  // Plain tags under Tags, then a row per namespace in the library's order,
  // each tag shown without its prefix.
  const tagGroups = new Map<
    string | null,
    { id: number; tagId: number; name: string }[]
  >([null, ...tagNamespaces].map((ns) => [ns, []]));
  for (const bt of book.book_tags ?? []) {
    const split = bt.tag
      ? splitTagNamespace(bt.tag.name, tagNamespaces)
      : { namespace: null, name: "Unknown" };
    tagGroups.get(split.namespace)!.push({
      id: bt.id,
      tagId: bt.tag_id,
      name: split.name,
    });
  }
  for (const [ns, tags] of tagGroups) {
    if (tags.length === 0) continue;
    rows.push({
      label: ns ?? "Tags",
      value: (
        <span className="flex flex-wrap gap-1.5">
          {tags.map((t) => (
            <Pill
              key={t.id}
              to={
                isShareLink
                  ? null
                  : `/libraries/${libraryId}?tag_ids=${t.tagId}`
              }
            >
              {t.name}
            </Pill>
          ))}
        </span>
      ),
    });
  }

  return (
    <div className="flex flex-col gap-6 md:flex-row">
      <div
        className={cn(
          "relative mx-auto w-40 shrink-0 self-start md:mx-0 md:w-[200px] lg:w-[220px]",
          coverVariant === "audiobook" ? "aspect-square" : "aspect-[2/3]",
        )}
      >
        {/* The placeholder shows until the image loads, or on an error. */}
        {(!coverLoaded || coverError || !coverUrl) && (
          <CoverPlaceholder
            className="absolute inset-0 rounded-md border border-border"
            variant={coverVariant}
          />
        )}
        {!coverError && coverUrl && (
          <CoverImage
            alt={`${book.title} Cover`}
            className={cn(
              "h-full w-full rounded-md border border-border object-cover",
              !coverLoaded && "opacity-0",
            )}
            key={coverUrl}
            onError={() => setCoverError(true)}
            onLoad={() => {
              markCoverLoaded(coverUrl);
              setCoverLoaded(true);
            }}
            src={coverUrl}
          />
        )}
        {coverBadge && (
          <div className="absolute bottom-2 left-2">{coverBadge}</div>
        )}
      </div>

      <div className="min-w-0 flex-1 space-y-4">
        <div>
          <h1 className="break-words text-3xl font-bold leading-tight">
            {book.title}
          </h1>
          {book.subtitle && (
            <p className="mt-1 break-words text-lg text-muted-foreground">
              {book.subtitle}
            </p>
          )}
          {people.length > 0 && (
            <p className="mt-1.5 break-words text-base">
              <span className="text-muted-foreground">by </span>
              {people.map((person, index) => (
                <span key={person.personId}>
                  <TextLink
                    to={
                      linkPeople && person.linked
                        ? `/libraries/${libraryId}/people/${person.personId}`
                        : null
                    }
                  >
                    {person.name}
                  </TextLink>
                  {/* Roles matter for comics, where a book credits a writer,
                      a penciller, and more. */}
                  {comic && person.roles.length > 0 && (
                    <span className="text-muted-foreground">
                      {" "}
                      ({person.roles.join(", ")})
                    </span>
                  )}
                  {index < people.length - 1 && ", "}
                </span>
              ))}
            </p>
          )}
        </div>
        {actions}
        {reading}
        <LabelGrid rows={rows} />
        {book.description && <FadedText text={book.description} />}
      </div>

      {sidePanel && (
        <aside className="hidden w-80 shrink-0 xl:block">{sidePanel}</aside>
      )}
    </div>
  );
};

export default BookDetailHeader;
