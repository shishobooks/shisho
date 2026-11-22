import { API } from "@/libraries/api";

// A file on the server read a part at a time with HTTP Range requests, so a
// reader never holds a whole book in memory. A browser tab on a phone is
// killed when it holds a few hundred MB, which a cookbook full of photos
// reaches; through this, opening one fetches its table of contents and the
// pages on screen.
//
// The download routes answer Range once the file is on disk. A Generated
// File still being built streams whole with no Accept-Ranges, so open()
// drops that answer and asks again until the build lands (it keeps running
// without the request, ADR 0010).

// What open() asks for: the end of the file, where a zip keeps its index.
const OPEN_TAIL = 64 * 1024;
// The least a read fetches, and the most a run of reads through one large
// entry grows to.
const MIN_FETCH = 256 * 1024;
const MAX_FETCH = 4 * 1024 * 1024;
// The most one read fetches for a caller that names where its run of reads
// ends, such as a zip entry; past it, reads grow as above.
const MAX_THROUGH = 16 * 1024 * 1024;
// How much of the file stays in memory once read.
const CACHE_LIMIT = 32 * 1024 * 1024;
const BUILD_POLL_FIRST = 250;
const BUILD_POLL_MAX = 2000;
// A proxy that strips both Range and Accept-Ranges looks like a build that
// never ends; after this long the caller downloads the file whole instead.
const BUILD_WAIT_LIMIT = 2 * 60 * 1000;
// Reads this small that start at once go out together, as one request for
// many ranges: foliate's MOBI reader reads four bytes of every resource
// record to find two of them, hundreds of reads across the whole file.
const TINY_READ = 64;
// Ranges per request, which keeps the Range header well under the 8 KB a
// proxy such as nginx allows a header line.
const MAX_RANGES = 64;
const READ_RETRIES = 2;
const READ_RETRY_DELAY = 500;

/** The server answered a Range request with the whole file. */
export class RangeNotSupportedError extends Error {
  constructor() {
    super("The server doesn't support reading part of a file.");
    this.name = "RangeNotSupportedError";
  }
}

/** The file on the server is no longer the copy that was opened. */
export class BookChangedError extends Error {
  constructor() {
    super(
      "This book changed on the server while it was open. Retry to load the new copy.",
    );
    this.name = "BookChangedError";
  }
}

interface Segment {
  start: number;
  end: number;
  data: Promise<Uint8Array>;
  settled: boolean;
}

const parseContentRange = (
  header: string | null,
): { start: number; end: number; total: number } | null => {
  const match = header && /^bytes (\d+)-(\d+)\/(\d+)$/.exec(header.trim());
  if (!match) return null;
  return {
    start: Number(match[1]),
    end: Number(match[2]) + 1,
    total: Number(match[3]),
  };
};

const CRLFCRLF = new Uint8Array([13, 10, 13, 10]);

const indexOf = (haystack: Uint8Array, needle: Uint8Array, from: number) => {
  outer: for (let i = from; i <= haystack.length - needle.length; i++) {
    for (let j = 0; j < needle.length; j++) {
      if (haystack[i + j] !== needle[j]) continue outer;
    }
    return i;
  }
  return -1;
};

interface Part {
  start: number;
  total: number;
  data: Uint8Array;
}

// The parts of a multipart/byteranges body, each with the range it holds.
const parseByteRanges = (body: Uint8Array, boundary: string): Part[] => {
  const delimiter = new TextEncoder().encode(`--${boundary}`);
  const decoder = new TextDecoder();
  const parts: Part[] = [];
  let at = indexOf(body, delimiter, 0);
  while (at !== -1) {
    const pos = at + delimiter.length;
    // "--" closes the body.
    if (body[pos] === 45 && body[pos + 1] === 45) break;
    const headerEnd = indexOf(body, CRLFCRLF, pos);
    if (headerEnd === -1) break;
    const headers = decoder.decode(body.subarray(pos, headerEnd));
    const range = parseContentRange(
      /content-range:\s*([^\r\n]*)/i.exec(headers)?.[1] ?? null,
    );
    if (!range) break;
    const dataStart = headerEnd + CRLFCRLF.length;
    const dataEnd = dataStart + range.end - range.start;
    parts.push({
      start: range.start,
      total: range.total,
      data: body.subarray(dataStart, dataEnd),
    });
    at = indexOf(body, delimiter, dataEnd);
  }
  return parts;
};

interface TinyRead {
  start: number;
  end: number;
  resolve: (data: Uint8Array) => void;
  reject: (error: unknown) => void;
}

const sleep = (ms: number, signal?: AbortSignal) =>
  new Promise<void>((resolve, reject) => {
    if (signal?.aborted) {
      reject(signal.reason);
      return;
    }
    const onAbort = () => {
      clearTimeout(timer);
      reject(signal?.reason);
    };
    const timer = setTimeout(() => {
      signal?.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    signal?.addEventListener("abort", onAbort, { once: true });
  });

// Fetches with a controller of its own, so an answer whose body isn't wanted
// can be dropped without reading it.
const fetchDroppable = async (
  url: string,
  headers: Record<string, string>,
  signal?: AbortSignal,
) => {
  const controller = new AbortController();
  const onAbort = () => controller.abort(signal?.reason);
  if (signal?.aborted) onAbort();
  signal?.addEventListener("abort", onAbort, { once: true });
  try {
    const response = await fetch(url, { headers, signal: controller.signal });
    return {
      response,
      drop: () => controller.abort(),
      done: () => signal?.removeEventListener("abort", onAbort),
    };
  } catch (error) {
    signal?.removeEventListener("abort", onAbort);
    throw error;
  }
};

export class RangeFile {
  readonly url: string;
  readonly size: number;
  // The Last-Modified (or ETag) of the copy opened, sent as If-Range so a
  // rebuilt file answers whole instead of mixing two copies.
  readonly #validator: string | null;
  #segments: Segment[] = [];
  #cached = 0;
  #changed: BookChangedError | null = null;
  #listeners = new Set<(error: BookChangedError) => void>();
  // The whole file, once downloadWhole() has it: reads come from here.
  #whole: Blob | null = null;
  #downloading: Promise<boolean> | null = null;
  // Tiny reads waiting to go out together, and whether the server answers
  // a request for several ranges with them.
  #tinyReads: TinyRead[] | null = null;
  #multiRange = true;

  private constructor(
    url: string,
    size: number,
    validator: string | null,
    tail: { start: number; data: Uint8Array },
  ) {
    this.url = url;
    this.size = size;
    this.#validator = validator;
    this.#keep(tail.start, tail.start + tail.data.length, tail.data);
  }

  /**
   * Opens the file at url, waiting while the server builds it. Rejects with
   * the API's error, or RangeNotSupportedError when the server sends the
   * file whole.
   */
  static async open(
    url: string,
    { signal }: { signal?: AbortSignal } = {},
  ): Promise<RangeFile> {
    const started = Date.now();
    let wait = BUILD_POLL_FIRST;
    for (;;) {
      const { response, drop, done } = await fetchDroppable(
        url,
        { Range: `bytes=-${OPEN_TAIL}` },
        signal,
      );
      try {
        if (response.status === 206) {
          const range = parseContentRange(
            response.headers.get("Content-Range"),
          );
          if (!range) {
            drop();
            throw new RangeNotSupportedError();
          }
          const data = new Uint8Array(await response.arrayBuffer());
          const validator =
            response.headers.get("ETag") ??
            response.headers.get("Last-Modified");
          return new RangeFile(url, range.total, validator, {
            start: range.start,
            data,
          });
        }
        if (!response.ok) await API.checkStatus(response);
        drop();
        if (
          response.headers.has("Accept-Ranges") ||
          Date.now() - started > BUILD_WAIT_LIMIT
        ) {
          throw new RangeNotSupportedError();
        }
      } finally {
        done();
      }
      await sleep(wait, signal);
      wait = Math.min(wait * 2, BUILD_POLL_MAX);
    }
  }

  /**
   * Downloads the whole file in the background, so later reads are local:
   * for a device that can hold it, where a jump across a cookbook would
   * otherwise fetch every photo in the chapter. Resolves whether the
   * download is in use. One that fails, is stopped, or is a different copy
   * from the one opened leaves reads going to the server.
   */
  downloadWhole({ signal }: { signal?: AbortSignal } = {}): Promise<boolean> {
    if (this.#whole) return Promise.resolve(true);
    if (!this.#downloading) {
      const attempt = this.#download(signal).finally(() => {
        if (this.#downloading === attempt) this.#downloading = null;
      });
      this.#downloading = attempt;
    }
    return this.#downloading;
  }

  async #download(signal?: AbortSignal): Promise<boolean> {
    try {
      const { response, drop, done } = await fetchDroppable(
        this.url,
        {},
        signal,
      );
      try {
        const validator =
          response.headers.get("ETag") ?? response.headers.get("Last-Modified");
        // A file being rebuilt streams without Accept-Ranges, and a rebuilt
        // one has a new validator: neither is the copy opened.
        if (
          response.status !== 200 ||
          !response.headers.has("Accept-Ranges") ||
          validator !== this.#validator
        ) {
          drop();
          return false;
        }
        // Chrome keeps a Blob this large on disk rather than in memory.
        const whole = await response.blob();
        if (whole.size !== this.size) return false;
        this.#whole = whole;
        // Everything read so far is in the download too.
        this.#segments = [];
        this.#cached = 0;
        return true;
      } finally {
        done();
      }
    } catch {
      return false;
    }
  }

  /** Calls listener once if the file on the server stops matching. */
  onChanged(listener: (error: BookChangedError) => void): () => void {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  }

  /**
   * Reads length bytes from offset, fetching what isn't in memory. A caller
   * about to read on to `through`, as zip.js reads an entry's header and
   * then its data 64 KB at a time, names it so one request covers the run.
   */
  async read(
    offset: number,
    length: number,
    through?: number,
  ): Promise<Uint8Array> {
    const end = Math.min(offset + length, this.size);
    if (end <= offset) return new Uint8Array(0);
    if (this.#changed) throw this.#changed;
    if (this.#whole) {
      return new Uint8Array(await this.#whole.slice(offset, end).arrayBuffer());
    }

    let segment = this.#segments.find((s) => s.start <= offset && s.end >= end);
    if (segment) {
      // Most recently used last, so eviction takes the oldest.
      this.#segments.splice(this.#segments.indexOf(segment), 1);
      this.#segments.push(segment);
      // The read ahead of an earlier entry often holds the start of this
      // one, so fetch the rest of the run now rather than in pieces as the
      // reads reach it. From offset, so one piece holds every later read.
      if (through !== undefined) {
        const runEnd = Math.min(through, this.size, offset + MAX_THROUGH);
        if (!this.#segments.some((s) => s.start <= offset && s.end >= runEnd)) {
          this.#fetch(offset, end, through);
        }
      }
    } else if (end - offset <= TINY_READ && through === undefined) {
      segment = this.#tiny(offset, end);
    } else {
      segment = this.#fetch(offset, end, through);
    }
    const data = await segment.data;
    // A copy: zip.js may hand the array to a stream that keeps it.
    return data.slice(offset - segment.start, end - segment.start);
  }

  #fetch(offset: number, end: number, through?: number): Segment {
    // A read that picks up where an earlier one stopped is reading through
    // a large entry 64 KB at a time, so each fetch doubles until it's big.
    const before = this.#segments.find(
      (s) => s.start < offset && s.end >= offset,
    );
    let ahead = before
      ? Math.min(
          MAX_FETCH,
          Math.max(MIN_FETCH, (before.end - before.start) * 2),
        )
      : MIN_FETCH;
    // A caller that names where its run ends, as a zip entry's reads do,
    // gets that run: the next entry is the next caller's, fetched alongside.
    if (through !== undefined) {
      ahead = Math.max(MIN_FETCH, Math.min(through - offset, MAX_THROUGH));
    }
    const fetchEnd = Math.min(this.size, Math.max(end, offset + ahead));
    return this.#forgetIfFailed(
      this.#keep(offset, fetchEnd, this.#fetchRange(offset, fetchEnd)),
    );
  }

  // Forgets a failed read, so reading the part again asks again.
  #forgetIfFailed(segment: Segment): Segment {
    segment.data.catch(() => {
      const at = this.#segments.indexOf(segment);
      if (at !== -1) {
        this.#segments.splice(at, 1);
        this.#cached -= segment.end - segment.start;
      }
    });
    return segment;
  }

  // A tiny read joins the others started in the same turn, which go out
  // together once it ends.
  #tiny(offset: number, end: number): Segment {
    if (!this.#multiRange) {
      return this.#forgetIfFailed(
        this.#keep(offset, end, this.#fetchRange(offset, end)),
      );
    }
    let read!: TinyRead;
    const data = new Promise<Uint8Array>((resolve, reject) => {
      read = { start: offset, end, resolve, reject };
    });
    if (!this.#tinyReads) {
      this.#tinyReads = [];
      queueMicrotask(() => this.#sendTinyReads());
    }
    this.#tinyReads.push(read);
    return this.#forgetIfFailed(this.#keep(offset, end, data));
  }

  #sendTinyReads() {
    const reads = (this.#tinyReads ?? []).sort((a, b) => a.start - b.start);
    this.#tinyReads = null;
    for (let i = 0; i < reads.length; i += MAX_RANGES) {
      const group = reads.slice(i, i + MAX_RANGES);
      this.#fetchRanges(group).then(
        (parts) => {
          for (const read of group) {
            const part = parts.find(
              (p) =>
                p.start <= read.start && p.start + p.data.length >= read.end,
            );
            if (part) {
              read.resolve(
                part.data.slice(read.start - part.start, read.end - part.start),
              );
            } else {
              read.reject(new TypeError("The server left out a range."));
            }
          }
        },
        (error: unknown) => {
          for (const read of group) read.reject(error);
        },
      );
    }
  }

  async #fetchRanges(reads: TinyRead[]): Promise<Part[]> {
    for (let attempt = 0; ; attempt++) {
      try {
        return await this.#fetchRangesOnce(reads);
      } catch (error) {
        const network = error instanceof TypeError;
        if (!network || attempt >= READ_RETRIES) throw error;
      }
      await sleep(READ_RETRY_DELAY * 2 ** attempt);
    }
  }

  async #fetchRangesOnce(reads: TinyRead[]): Promise<Part[]> {
    if (!this.#multiRange || reads.length === 1) {
      return Promise.all(
        reads.map(async ({ start, end }) => ({
          start,
          total: this.size,
          data: await this.#fetchRange(start, end),
        })),
      );
    }
    const headers: Record<string, string> = {
      Range: `bytes=${reads.map(({ start, end }) => `${start}-${end - 1}`).join(",")}`,
    };
    if (this.#validator) headers["If-Range"] = this.#validator;
    const { response, drop, done } = await fetchDroppable(this.url, headers);
    try {
      if (response.status === 206) {
        const type = response.headers.get("Content-Type") ?? "";
        const boundary = /boundary=("?)([^";]+)\1/i.exec(type)?.[2];
        const body = new Uint8Array(await response.arrayBuffer());
        // A server may answer with one range that holds them all.
        const single = parseContentRange(response.headers.get("Content-Range"));
        const parts = boundary
          ? parseByteRanges(body, boundary)
          : single
            ? [{ start: single.start, total: single.total, data: body }]
            : [];
        if (parts.some((part) => part.total !== this.size)) {
          throw this.#markChanged();
        }
        return parts;
      }
      if (!response.ok) await API.checkStatus(response);
      drop();
      const validator =
        response.headers.get("ETag") ?? response.headers.get("Last-Modified");
      // The copy opened, sent whole: the server takes one range at a time.
      if (
        response.headers.has("Accept-Ranges") &&
        validator === this.#validator
      ) {
        this.#multiRange = false;
        return this.#fetchRangesOnce(reads);
      }
      throw this.#markChanged();
    } finally {
      done();
    }
  }

  #keep(
    start: number,
    end: number,
    data: Uint8Array | Promise<Uint8Array>,
  ): Segment {
    const segment: Segment = {
      start,
      end,
      data: Promise.resolve(data),
      settled: !(data instanceof Promise),
    };
    if (data instanceof Promise) {
      data.then(
        () => (segment.settled = true),
        () => (segment.settled = true),
      );
    }
    this.#segments.push(segment);
    this.#cached += end - start;
    for (let i = 0; this.#cached > CACHE_LIMIT && i < this.#segments.length;) {
      const oldest = this.#segments[i];
      // A fetch in flight has readers waiting on it.
      if (!oldest.settled || oldest === segment) {
        i++;
        continue;
      }
      this.#segments.splice(i, 1);
      this.#cached -= oldest.end - oldest.start;
    }
    return segment;
  }

  async #fetchRange(start: number, end: number): Promise<Uint8Array> {
    const headers: Record<string, string> = {
      Range: `bytes=${start}-${end - 1}`,
    };
    if (this.#validator) headers["If-Range"] = this.#validator;
    for (let attempt = 0; ; attempt++) {
      try {
        return await this.#fetchOnce(headers, start, end);
      } catch (error) {
        // A dropped connection gets another try; the server's answer
        // doesn't.
        const network = error instanceof TypeError;
        if (!network || attempt >= READ_RETRIES) throw error;
      }
      await sleep(READ_RETRY_DELAY * 2 ** attempt);
    }
  }

  async #fetchOnce(
    headers: Record<string, string>,
    start: number,
    end: number,
  ): Promise<Uint8Array> {
    const { response, drop, done } = await fetchDroppable(this.url, headers);
    try {
      if (response.status === 206) {
        const range = parseContentRange(response.headers.get("Content-Range"));
        if (!range || range.start !== start || range.total !== this.size) {
          drop();
          throw this.#markChanged();
        }
        const data = new Uint8Array(await response.arrayBuffer());
        // Short, as a cut connection leaves it: worth another try.
        if (range.end < end || data.length !== range.end - start) {
          throw new TypeError("The server sent part of the range.");
        }
        return data.subarray(0, end - start);
      }
      if (!response.ok) await API.checkStatus(response);
      // A whole file for an If-Range request is a different copy: rebuilt,
      // or being rebuilt.
      drop();
      throw this.#markChanged();
    } finally {
      done();
    }
  }

  #markChanged(): BookChangedError {
    if (!this.#changed) {
      this.#changed = new BookChangedError();
      for (const listener of this.#listeners) listener(this.#changed);
    }
    return this.#changed;
  }
}
