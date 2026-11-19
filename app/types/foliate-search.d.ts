// The vendored search.js and text-walker.js, which foliate's view.search()
// uses to find a query in a section's document.
declare module "@/libraries/foliate/search.js" {
  export function searchMatcher(
    textWalker: unknown,
    opts: {
      defaultLocale?: string;
      matchCase?: boolean;
      matchDiacritics?: boolean;
      matchWholeWords?: boolean;
    },
  ): (
    doc: Document,
    query: string,
  ) => Generator<{
    range: Range;
    excerpt: { pre: string; match: string; post: string };
  }>;
}

declare module "@/libraries/foliate/text-walker.js" {
  export const textWalker: unknown;
}
