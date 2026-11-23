// Types for what Shisho uses of the vendored app/libraries/foliate/mobi.js:
// a Shisho patch's helper, and the reader app/libraries/rangeMobi.ts opens
// a book with.
declare module "@/libraries/foliate/mobi.js" {
  export function removeUnresolvedSrcset(doc: Document): void;

  // A parsed MOBI or AZW3, which <foliate-view>'s open() takes in place of
  // a file.
  export interface FoliateMOBI {
    metadata: { title?: string; language?: string };
    // A section foliate doesn't show (linear: "no") has no load.
    sections: Array<{
      load?: () => Promise<string>;
      createDocument(): Promise<Document>;
    }>;
  }

  export class MOBI {
    constructor(options: { unzlib: (data: Uint8Array) => Uint8Array });
    open(file: {
      size: number;
      slice(start?: number, end?: number): unknown;
    }): Promise<FoliateMOBI>;
  }
}

declare module "@/libraries/foliate/vendor/fflate.js" {
  export function unzlibSync(data: Uint8Array): Uint8Array;
}
