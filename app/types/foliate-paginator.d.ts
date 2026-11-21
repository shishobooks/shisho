// The part of app/libraries/foliate/paginator.js its patch test reads.
declare module "@/libraries/foliate/paginator.js" {
  export class Paginator extends HTMLElement {
    // A Shisho patch: resolves once no page turn is in progress.
    settled(): Promise<void>;
  }
}
