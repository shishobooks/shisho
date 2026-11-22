import type { FoliateEPUB } from "@/libraries/foliate/epub.js";
import type { RangeFile } from "@/libraries/rangeFile";

// The fixed part of a zip entry's local file header.
const LOCAL_HEADER_SIZE = 30;
// How long a resource fetched ahead waits for foliate to ask for it. foliate
// asks within moments of a chapter loading; one it never asks for, such as
// a font its loader already holds for another chapter, is dropped after
// this.
const PREFETCH_KEEP_MS = 30_000;
const XLINK = "http://www.w3.org/1999/xlink";

// The path in the book that ref, written in the document at base, names:
// how foliate resolves it (resolveURL in epub.js), so the two agree.
const resolveHref = (ref: string, base: string): string | null => {
  const root = "https://invalid.invalid/";
  try {
    const url = new URL(ref, root + base);
    if (url.origin !== new URL(root).origin) return null;
    return decodeURI(url.pathname.slice(1));
  } catch {
    return null;
  }
};

// What foliate rewrites, loading what it names, rather than reading whole
// (shouldReplace in epub.js's Loader).
const REWRITTEN =
  /^(application\/xhtml\+xml|text\/html|text\/css|image\/svg\+xml)$|\/(x-)?(javascript|ecmascript)/;

const resolveAll = (refs: string[], base: string): string[] =>
  refs
    .filter((ref) => ref !== "")
    .map((ref) => resolveHref(ref, base))
    .filter((href): href is string => href !== null);

// What a stylesheet's url()s name, as foliate's replaceCSS finds them.
const stylesheetHrefs = (css: string, base: string): string[] =>
  resolveAll(
    [...css.matchAll(/url\(\s*["']?([^'"\n]*?)\s*["']?\s*\)/gi)].map(
      (match) => match[1],
    ),
    base,
  );

// The resources a chapter's document points at that foliate loads with it.
const documentHrefs = (text: string, base: string): string[] => {
  let doc = new DOMParser().parseFromString(text, "application/xhtml+xml");
  if (doc.querySelector("parsererror")) {
    doc = new DOMParser().parseFromString(text, "text/html");
  }
  const refs: string[] = [];
  for (const el of doc.querySelectorAll("[src], [poster], object[data]")) {
    refs.push(
      el.getAttribute("src") ??
        el.getAttribute("poster") ??
        el.getAttribute("data") ??
        "",
    );
  }
  for (const el of doc.querySelectorAll("image")) {
    refs.push(
      el.getAttributeNS(XLINK, "href") ?? el.getAttribute("href") ?? "",
    );
  }
  for (const el of doc.querySelectorAll("[srcset]")) {
    for (const candidate of el.getAttribute("srcset")!.split(",")) {
      refs.push(candidate.trim().split(/\s+/)[0]);
    }
  }
  return resolveAll(refs, base);
};

/**
 * Opens an EPUB a part at a time from the server, for <foliate-view>'s
 * open(). The same loader foliate's makeBook builds from a whole Blob
 * (makeZipLoader in view.js), reading through a RangeFile instead. foliate's
 * modules load on first use, as view.js loads them, so they stay out of the
 * app's own chunks.
 */
export const openRangeEPUB = async (file: RangeFile): Promise<FoliateEPUB> => {
  const [
    { EPUB },
    { BlobReader, BlobWriter, configure, TextWriter, ZipReader },
  ] = await Promise.all([
    import("@/libraries/foliate/epub.js"),
    import("@/libraries/foliate/vendor/zip.js"),
  ]);

  // Where each entry ends, by where it starts. zip.js reads an entry's
  // header at its offset and then its data, so a read at an entry's offset
  // fetches the whole entry: a photo in one request rather than several.
  const entryEnds = new Map<number, number>();

  // zip.js reads through any reader with a size and readUint8Array, but
  // reading an entry also needs the `readable` its own Reader class provides,
  // so this takes BlobReader's and reads the bytes from the server.
  class RangeReader extends BlobReader {
    constructor() {
      super(new Blob());
      this.size = file.size;
    }

    override readUint8Array(offset: number, length: number) {
      return file.read(offset, length, entryEnds.get(offset));
    }
  }

  configure({ useWebWorkers: false });
  const reader = new ZipReader(new RangeReader());
  const entries = await reader.getEntries();
  for (const entry of entries) {
    // The local header repeats the name and usually the extra field; one
    // that differs leaves a little to read after the fetch.
    entryEnds.set(
      entry.offset,
      entry.offset +
        LOCAL_HEADER_SIZE +
        entry.rawFilename.length +
        entry.rawExtraField.length +
        entry.compressedSize,
    );
  }
  const map = new Map(entries.map((entry) => [entry.filename, entry]));

  // foliate's loader asks for the resources a chapter or stylesheet names
  // one after another, waiting for each (Loader.loadReplaced and replaceCSS
  // in epub.js). From memory that cost nothing; from the server it's a round
  // trip each, and a cookbook chapter has dozens of photos and its
  // stylesheet thirty fonts. So when foliate starts on a chapter or a
  // stylesheet, everything it names is fetched at once, and each of
  // foliate's asks takes the one already on its way.
  const prefetched = new Map<string, Promise<Blob>>();
  // What foliate reads whole rather than rewriting (photos, fonts), and the
  // stylesheets it rewrites; known once the book's manifest is.
  let leaves = new Set<string>();
  let stylesheets = new Set<string>();
  const prefetch = (hrefs: string[]) => {
    for (const href of hrefs) {
      const entry = map.get(href);
      if (!leaves.has(href) || !entry || prefetched.has(href)) continue;
      const blob = entry.getData(new BlobWriter());
      prefetched.set(href, blob);
      const drop = () =>
        setTimeout(() => {
          if (prefetched.get(href) === blob) prefetched.delete(href);
        }, PREFETCH_KEEP_MS);
      blob.then(drop, drop);
    }
  };

  const loadText = (name: string) => {
    const text = map.get(name)?.getData(new TextWriter()) ?? null;
    // foliate reads a stylesheet's text only to rewrite it, which loads
    // what it names.
    if (!text || !stylesheets.has(name)) return text;
    return text.then((css) => {
      prefetch(stylesheetHrefs(css, name));
      return css;
    });
  };
  const loadBlob = (name: string, type?: string) => {
    const early = prefetched.get(name);
    if (early) {
      prefetched.delete(name);
      return early;
    }
    return map.get(name)?.getData(new BlobWriter(type)) ?? null;
  };
  const getSize = (name: string) => map.get(name)?.uncompressedSize ?? 0;
  const book = await new EPUB({ loadText, loadBlob, getSize }).init();

  const manifest = book.resources.manifest;
  leaves = new Set(
    manifest
      .filter(({ mediaType }) => !REWRITTEN.test(mediaType))
      .map(({ href }) => href),
  );
  stylesheets = new Set(
    manifest
      .filter(({ mediaType }) => mediaType === "text/css")
      .map(({ href }) => href),
  );
  // foliate also reads a chapter's text to search it or measure it, which
  // loads nothing it names, so a chapter's resources are fetched when its
  // page is about to show.
  for (const section of book.sections) {
    const load = section.load;
    section.load = async () => {
      // Fetching ahead only speeds the load up; foliate's own asks still
      // fetch anything it missed.
      try {
        const text = await map.get(section.id)?.getData(new TextWriter());
        if (text) prefetch(documentHrefs(text, section.id));
      } catch {
        // foliate's load reports it.
      }
      return load();
    };
  }
  return book;
};
