// The part of app/libraries/foliate/paginator.js its patch tests read.
declare module "@/libraries/foliate/paginator.js" {
  export class Paginator extends HTMLElement {
    // A Shisho patch: resolves once no page turn is in progress.
    settled(): Promise<void>;
    // A Shisho patch: the page going to an element or range would show.
    pageOf(anchor: Element | Range): number | null;
  }
  // A Shisho patch: clears forced breaks before a section's first content.
  export function clearLeadingBreaks(doc: Document): void;
}
