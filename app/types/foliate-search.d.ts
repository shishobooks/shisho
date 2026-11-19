// Types for the drawing functions of the vendored
// app/libraries/foliate/overlayer.js, which foliate uses to mark ranges in
// a book, such as search matches.
declare module "@/libraries/foliate/overlayer.js" {
  export type OverlayerDraw = (
    rects: DOMRectList | DOMRect[],
    options?: Record<string, unknown>,
  ) => SVGElement;

  export class Overlayer {
    static highlight: OverlayerDraw;
    static outline: OverlayerDraw;
  }
}

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
