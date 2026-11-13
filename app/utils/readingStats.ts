import type {
  ReadingKind,
  ReadingSource,
  ReadingStatsCount,
  ReadingStatsKind,
  ReadingStatsKindCount,
  ReadingStatsPlace,
  ReadingStatsRatingCount,
} from "@/types";
import { formatCount } from "@/utils/format";

// The rows of the Reading Stats breakdowns, from the API's counts.

/** One row of a breakdown: a name and how many finished books it counts. */
export interface CountRow {
  /** What selecting the row filters the finished books by. */
  value: string;
  label: string;
  books: number;
}

export const countRows = (counts: ReadingStatsCount[]): CountRow[] =>
  counts.map((count) => ({
    value: count.name,
    label: count.name,
    books: count.books,
  }));

export const KIND_LABELS: Record<ReadingStatsKind, string> = {
  ebook: "Ebooks",
  comic: "Comics",
  audiobook: "Audiobooks",
  unknown: "Unknown",
};

export const kindRows = (kinds: ReadingStatsKindCount[]): CountRow[] =>
  kinds.map((kind) => ({
    value: kind.kind,
    label: KIND_LABELS[kind.kind],
    books: kind.books,
  }));

export const ratingLabel = (stars: number) => formatCount(stars, "star");

// Highest first, as a reader scans a rating breakdown.
export const ratingRows = (ratings: ReadingStatsRatingCount[]): CountRow[] =>
  [...ratings]
    .sort((a, b) => b.rating - a.rating)
    .map((rating) => ({
      value: String(rating.rating),
      label: ratingLabel(rating.rating),
      books: rating.books,
    }));

// What each reading source is called. Shisho's own are named as Shisho's,
// apart from third-party devices and readers; "web" leaves room for other
// apps of Shisho's. The web app has a reader and a player, named apart.
const SOURCE_LABELS: Record<ReadingSource, string> = {
  web_reader: "Shisho web reader",
  kobo: "Kobo",
  koreader: "KOReader",
};
const WEB_PLAYER = "Shisho web player";

/**
 * Where a Reading Session happened: "Shisho web reader", "Shisho web
 * player" for listening in the web app, or the device's kind, marked when
 * its time was estimated.
 */
export const sessionSourceLabel = (session: {
  source: ReadingSource;
  kind: ReadingKind;
  measured: boolean;
}): string => {
  const name =
    session.source === "web_reader" && session.kind === "listening"
      ? WEB_PLAYER
      : SOURCE_LABELS[session.source];
  return withEstimate(name, session.measured);
};

/** A session's place, marked when its time was estimated. */
export const withEstimate = (place: string, measured: boolean): string =>
  measured ? place : `${place} (estimated)`;

/** One row of "Where you read". */
export interface PlaceRow {
  label: string;
  reading_seconds: number;
  listening_seconds: number;
}

/**
 * The rows of "Where you read", most time first: the web app split into
 * its reader and its player, each shown only with time, and each device by
 * its name, or by its kind once it's removed.
 */
export const placeRows = (places: ReadingStatsPlace[]): PlaceRow[] =>
  places
    .flatMap((place): PlaceRow[] => {
      if (place.source !== "web_reader") {
        return [
          {
            label: place.device_name ?? SOURCE_LABELS[place.source],
            reading_seconds: place.reading_seconds,
            listening_seconds: place.listening_seconds,
          },
        ];
      }
      return [
        {
          label: SOURCE_LABELS.web_reader,
          reading_seconds: place.reading_seconds,
          listening_seconds: 0,
        },
        {
          label: WEB_PLAYER,
          reading_seconds: 0,
          listening_seconds: place.listening_seconds,
        },
      ].filter((row) => row.reading_seconds + row.listening_seconds > 0);
    })
    .sort(
      (a, b) =>
        b.reading_seconds +
        b.listening_seconds -
        (a.reading_seconds + a.listening_seconds),
    );
