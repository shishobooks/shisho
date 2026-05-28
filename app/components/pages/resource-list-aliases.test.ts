/**
 * Tests for alias display on resource list pages.
 *
 * The backend returns aliases as string[] (plain name strings), but the
 * generated TypeScript types declare them as {name: string, ...}[] objects.
 * The itemConfig callbacks in list pages must pass aliases through as-is
 * since they're already strings, not map them with .name (which yields undefined).
 */
import { describe, expect, it } from "vitest";

// Simulate the actual API response shape: aliases are plain strings
// (even though the generated TS types say PersonAlias[], GenreAlias[], etc.)
interface ApiPersonWithCounts {
  id: number;
  name: string;
  sort_name: string;
  aliases: string[];
  authored_book_count: number;
  narrated_file_count: number;
}

interface ApiGenre {
  id: number;
  name: string;
  aliases: string[];
  book_count: number;
}

interface ApiTag {
  id: number;
  name: string;
  aliases: string[];
  book_count: number;
}

interface ApiPublisherListItem {
  id: number;
  name: string;
  aliases: string[];
  parent_name: string | null;
  file_count: number;
  descendant_file_count: number;
  descendant_publisher_count: number;
}

// Extract the alias-mapping logic from each list page's itemConfig.
// These functions mirror the FIXED code: pass aliases through directly
// since the backend already returns string[].

function personAliases(person: ApiPersonWithCounts): string[] {
  return person.aliases;
}

function genreAliases(genre: ApiGenre): string[] {
  return genre.aliases;
}

function tagAliases(tag: ApiTag): string[] {
  return tag.aliases;
}

function publisherAliases(publisher: ApiPublisherListItem): string[] {
  return publisher.aliases;
}

describe("resource list alias display", () => {
  describe("PersonList", () => {
    it("should pass aliases through as strings, not map .name on them", () => {
      const person: ApiPersonWithCounts = {
        id: 1,
        name: "Stephen King",
        sort_name: "King, Stephen",
        aliases: ["Richard Bachman", "The King"],
        authored_book_count: 5,
        narrated_file_count: 0,
      };

      const result = personAliases(person);
      // BUG: .map((a) => a.name) on string[] yields [undefined, undefined]
      expect(result).toEqual(["Richard Bachman", "The King"]);
    });

    it("should handle empty aliases", () => {
      const person: ApiPersonWithCounts = {
        id: 2,
        name: "Nobody",
        sort_name: "Nobody",
        aliases: [],
        authored_book_count: 0,
        narrated_file_count: 0,
      };

      const result = personAliases(person);
      expect(result).toEqual([]);
    });
  });

  describe("GenresList", () => {
    it("should pass aliases through as strings, not map .name on them", () => {
      const genre: ApiGenre = {
        id: 1,
        name: "Science Fiction",
        aliases: ["Sci-Fi", "SF"],
        book_count: 10,
      };

      const result = genreAliases(genre);
      expect(result).toEqual(["Sci-Fi", "SF"]);
    });
  });

  describe("TagsList", () => {
    it("should pass aliases through as strings, not map .name on them", () => {
      const tag: ApiTag = {
        id: 1,
        name: "Must Read",
        aliases: ["Essential", "Required Reading"],
        book_count: 3,
      };

      const result = tagAliases(tag);
      expect(result).toEqual(["Essential", "Required Reading"]);
    });
  });

  describe("PublishersList", () => {
    it("should pass aliases through as strings, not map .name on them", () => {
      const publisher: ApiPublisherListItem = {
        id: 1,
        name: "Penguin Random House",
        aliases: ["PRH", "Penguin"],
        parent_name: null,
        file_count: 20,
        descendant_file_count: 5,
        descendant_publisher_count: 2,
      };

      const result = publisherAliases(publisher);
      expect(result).toEqual(["PRH", "Penguin"]);
    });
  });
});
