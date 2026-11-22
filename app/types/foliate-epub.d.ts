// The parts of the vendored app/libraries/foliate/epub.js and
// vendor/zip.js that app/libraries/rangeEpub.ts builds a book from.
declare module "@/libraries/foliate/epub.js" {
  export interface FoliateSection {
    id: string;
    size: number;
    load(): Promise<string>;
    createDocument(): Promise<Document>;
  }

  // A parsed book, which <foliate-view>'s open() takes in place of a file.
  export interface FoliateEPUB {
    metadata: { title?: string; language?: string };
    sections: FoliateSection[];
    resources: { manifest: Array<{ href: string; mediaType: string }> };
    loadBlob(name: string): Promise<Blob | null> | Blob | null;
    destroy(): void;
  }

  export class EPUB {
    constructor(loader: {
      loadText: (name: string) => Promise<string> | null;
      loadBlob: (name: string, type?: string) => Promise<Blob> | null;
      getSize: (name: string) => number;
    });
    init(): Promise<FoliateEPUB>;
  }
}

declare module "@/libraries/foliate/vendor/zip.js" {
  export function configure(options: { useWebWorkers?: boolean }): void;

  export class BlobReader {
    constructor(blob: Blob);
    size: number;
    readUint8Array(offset: number, length: number): Promise<Uint8Array>;
  }

  // Distinct, so getData's overloads tell them apart.
  export class TextWriter {
    private readonly text: never;
  }

  export class BlobWriter {
    constructor(type?: string);
    private readonly blob: never;
  }

  export interface ZipEntry {
    filename: string;
    uncompressedSize: number;
    // Where its local header starts, and the sizes that follow it.
    offset: number;
    compressedSize: number;
    rawFilename: Uint8Array;
    rawExtraField: Uint8Array;
    getData(writer: TextWriter): Promise<string>;
    getData(writer: BlobWriter): Promise<Blob>;
  }

  export class ZipReader {
    constructor(reader: BlobReader);
    getEntries(): Promise<ZipEntry[]>;
  }
}
