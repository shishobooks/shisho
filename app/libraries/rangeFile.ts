import { API, ShishoAPIError } from "@/libraries/api";

// A file on the server read a part at a time with HTTP Range requests, so a
// reader never holds a whole book in memory. A browser tab on a phone is
// killed when it holds a few hundred MB, which a cookbook full of photos
// reaches; through this, opening one fetches its table of contents and the
// pages on screen.
//
// The download routes answer Range once the file is on disk. A Generated
// File still being built streams whole with neither Accept-Ranges nor
// Last-Modified (httputil.ServeStream), so open() drops that answer and asks
// again until the build lands (it keeps running without the request, ADR
// 0010). A finished file always has Last-Modified, so a 200 with it is a
// server or proxy ignoring Range.

// What open() asks for: the end of the file, where a zip keeps its index.
const OPEN_TAIL = 64 * 1024;
// The least a read fetches, and the most a run of reads through one large
// entry grows to.
const MIN_FETCH = 256 * 1024;
const MAX_FETCH = 4 * 1024 * 1024;
// The most one read fetches for a caller that names where its run of reads
// ends, such as a zip entry; past it, reads grow as above.
const MAX_THROUGH = 16 * 1024 * 1024;
// How much of the file stays in memory once read. A part fetched for one
// read, or one run of reads, is dropped as soon as they have it.
const CACHE_LIMIT = 32 * 1024 * 1024;
// Requests at once. Each holds its part in memory until it lands, so a
// chapter of fifty photos asked for together arrives a few at a time; it's
// also what a browser allows a server over HTTP/1.1.
const MAX_REQUESTS = 6;
const BUILD_POLL_FIRST = 250;
const BUILD_POLL_MAX = 2000;
// A build still running after this long is reported rather than waited on.
const BUILD_WAIT_LIMIT = 10 * 60 * 1000;
// Reads this small that start at once go out together, as one request for
// many ranges: foliate's MOBI reader reads four bytes of every resource
// record to find two of them, hundreds of reads across the whole file.
const TINY_READ = 64;
// Ranges per request, which keeps the Range header well under the 8 KB a
// proxy such as nginx allows a header line.
const MAX_RANGES = 64;
const RETRIES = 2;
const RETRY_DELAY = 500;
// Statuses that come and go: a proxy whose upstream restarted, a busy server.
const RETRY_STATUSES = new Set([408, 429, 502, 503, 504]);

/** The server answered a Range request for url with the whole file. */
export class RangeNotSupportedError extends Error {
  readonly url: string;

  constructor(url: string) {
    super("The server doesn't support reading part of a file.");
    this.name = "RangeNotSupportedError";
    this.url = url;
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

/** The server was still building the file after BUILD_WAIT_LIMIT. */
export class BuildTimeoutError extends Error {
  constructor() {
    super(
      "This book is taking a long time to prepare. Try again in a few minutes.",
    );
    this.name = "BuildTimeoutError";
  }
}

// An answer that's worth asking for again: cut short, or framed wrongly.
class BrokenAnswerError extends TypeError {
  constructor(message: string) {
    super(message);
    this.name = "BrokenAnswerError";
  }
}

interface Segment {
  start: number;
  end: number;
  data: Promise<Uint8Array>;
  settled: boolean;
  // Fetched for one read or one run of reads, and dropped once read to its
  // end, rather than kept for reads nearby.
  run: boolean;
}

// By name: an abort's DOMException can come from another realm.
const isAbort = (error: unknown) =>
  (error as { name?: unknown } | null)?.name === "AbortError";

const isRetryable = (error: unknown) =>
  (error instanceof TypeError && !isAbort(error)) ||
  (error instanceof ShishoAPIError && RETRY_STATUSES.has(error.status));

// The version of the file a response holds, as If-Range takes it.
const validatorOf = (response: Response) =>
  response.headers.get("ETag") ?? response.headers.get("Last-Modified");

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

const CRLF = new Uint8Array([13, 10]);
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

const startsWith = (haystack: Uint8Array, needle: Uint8Array, at: number) =>
  indexOf(haystack.subarray(at, at + needle.length), needle, 0) === 0;

interface Part {
  start: number;
  total: number;
  data: Uint8Array;
}

// The parts of a multipart/byteranges body, each with the range it holds.
// Each part's data must end where the next delimiter starts, so a part cut
// short throws rather than handing back the framing as data.
const parseByteRanges = (body: Uint8Array, boundary: string): Part[] => {
  const delimiter = new TextEncoder().encode(`--${boundary}`);
  const decoder = new TextDecoder();
  const parts: Part[] = [];
  let at = indexOf(body, delimiter, 0);
  if (at === -1) throw new BrokenAnswerError("No multipart delimiter.");
  for (;;) {
    const pos = at + delimiter.length;
    // "--" closes the body; a part's headers start on the next line.
    if (body[pos] === 45 && body[pos + 1] === 45) return parts;
    if (!startsWith(body, CRLF, pos)) {
      throw new BrokenAnswerError("A multipart delimiter is malformed.");
    }
    const headerEnd = indexOf(body, CRLFCRLF, pos);
    const range =
      headerEnd === -1
        ? null
        : parseContentRange(
            /content-range:\s*([^\r\n]*)/i.exec(
              decoder.decode(body.subarray(pos, headerEnd)),
            )?.[1] ?? null,
          );
    if (!range) throw new BrokenAnswerError("A multipart part has no range.");
    const dataStart = headerEnd + CRLFCRLF.length;
    const dataEnd = dataStart + range.end - range.start;
    if (
      !startsWith(body, CRLF, dataEnd) ||
      !startsWith(body, delimiter, dataEnd + CRLF.length)
    ) {
      throw new BrokenAnswerError("A multipart part was cut short.");
    }
    parts.push({
      start: range.start,
      total: range.total,
      data: body.subarray(dataStart, dataEnd),
    });
    at = dataEnd + CRLF.length;
  }
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

const withRetries = async <T>(
  attempt: () => Promise<T>,
  signal: AbortSignal,
): Promise<T> => {
  for (let tries = 0; ; tries++) {
    try {
      return await attempt();
    } catch (error) {
      if (!isRetryable(error) || tries >= RETRIES) throw error;
    }
    await sleep(RETRY_DELAY * 2 ** tries, signal);
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
  #listeners = new Set<(error: Error) => void>();
  // The whole file, once downloadWhole() has it: reads come from here.
  #whole: Blob | null = null;
  #downloading: Promise<boolean> | null = null;
  // Tiny reads waiting to go out together, and whether the server answers
  // a request for several ranges with them.
  #tinyReads: TinyRead[] | null = null;
  #multiRange = true;
  // Requests going and waiting to go (MAX_REQUESTS).
  #requests = 0;
  #queued: Array<() => void> = [];
  // Stops the reads of an opening the reader has left (cancelPending).
  #cancel = new AbortController();

  private constructor(
    url: string,
    size: number,
    validator: string | null,
    tail: { start: number; data: Uint8Array },
  ) {
    this.url = url;
    this.size = size;
    this.#validator = validator;
    this.#keep(tail.start, tail.start + tail.data.length, tail.data, false);
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
          // The end of the file, as asked for.
          if (!range || range.end !== range.total) {
            drop();
            throw new BrokenAnswerError("The server sent another range.");
          }
          const data = new Uint8Array(await response.arrayBuffer());
          if (data.length !== range.end - range.start) {
            throw new BrokenAnswerError("The server sent part of the range.");
          }
          return new RangeFile(url, range.total, validatorOf(response), {
            start: range.start,
            data,
          });
        }
        if (!response.ok) await API.checkStatus(response);
        drop();
        // A finished file sent whole: Range was ignored, or stripped on
        // the way.
        if (response.headers.has("Accept-Ranges") || validatorOf(response)) {
          throw new RangeNotSupportedError(url);
        }
        if (Date.now() - started > BUILD_WAIT_LIMIT) {
          throw new BuildTimeoutError();
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
        // A file being rebuilt streams without Accept-Ranges, and a rebuilt
        // one has a new validator: neither is the copy opened.
        if (
          response.status !== 200 ||
          !response.headers.has("Accept-Ranges") ||
          validatorOf(response) !== this.#validator
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

  /**
   * Calls listener when the book can't go on: once when the copy on the
   * server changes (BookChangedError), whichever read finds it, and for each
   * failure a reader passes to reportFailure().
   */
  onFailed(listener: (error: Error) => void): () => void {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  }

  /**
   * Tells the listeners a read the book needed failed for good, such as a
   * page's chapter. Not for reads made ahead, whose failure a later read
   * may not repeat, nor for reads cancelPending() stopped.
   */
  reportFailure(error: unknown) {
    if (isAbort(error) || error instanceof BookChangedError) return;
    this.#notify(error instanceof Error ? error : new Error(String(error)));
  }

  #notify(error: Error) {
    for (const listener of this.#listeners) listener(error);
  }

  /**
   * Stops the reads under way, for a reader that closed the book or opened
   * it again: their requests and the waits between tries end, and they
   * reject as aborted. Later reads go ahead.
   */
  cancelPending() {
    this.#cancel.abort();
    this.#cancel = new AbortController();
    // A read made from here on asks again rather than joining one stopped.
    for (const segment of this.#segments.filter((s) => !s.settled)) {
      this.#drop(segment);
    }
    this.#tinyReads = null;
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
    // A part fetched for this read, or read to its end by the last read of
    // its run, has nothing left to give.
    if (segment.run && end >= segment.end) this.#drop(segment);
    // A copy: zip.js may hand the array to a stream that keeps it.
    return data.slice(offset - segment.start, end - segment.start);
  }

  #fetch(offset: number, end: number, through?: number): Segment {
    // A read that picks up where an earlier one stopped is reading through
    // a large entry 64 KB at a time, so each fetch doubles until it's big.
    // foliate reads a MOBI's text from its end too, a record at a time
    // backward, so a read that ends where an earlier one started grows the
    // same way toward the start of the file.
    const before = this.#segments.find(
      (s) => s.start < offset && s.end >= offset,
    );
    const after = this.#segments.find((s) => s.start === end);
    const grow = (from: Segment | undefined) =>
      from
        ? Math.min(MAX_FETCH, Math.max(MIN_FETCH, (from.end - from.start) * 2))
        : MIN_FETCH;
    // A caller that names where its run ends, as a zip entry's reads do,
    // gets that run: the next entry is the next caller's, fetched alongside.
    if (through !== undefined) {
      const ahead = Math.max(
        MIN_FETCH,
        Math.min(through - offset, MAX_THROUGH),
      );
      const fetchEnd = Math.min(this.size, Math.max(end, offset + ahead));
      return this.#request(offset, fetchEnd, true);
    }
    if (after && !before) {
      const fetchStart = Math.max(0, Math.min(offset, end - grow(after)));
      return this.#request(fetchStart, end, fetchStart === offset);
    }
    const fetchEnd = Math.min(this.size, Math.max(end, offset + grow(before)));
    return this.#request(offset, fetchEnd, fetchEnd === end);
  }

  #request(start: number, end: number, run: boolean): Segment {
    return this.#forgetIfFailed(
      this.#keep(start, end, this.#fetchRange(start, end), run),
    );
  }

  // Forgets a failed read, so reading the part again asks again.
  #forgetIfFailed(segment: Segment): Segment {
    segment.data.catch(() => this.#drop(segment));
    return segment;
  }

  #drop(segment: Segment) {
    const at = this.#segments.indexOf(segment);
    if (at !== -1) {
      this.#segments.splice(at, 1);
      this.#cached -= segment.end - segment.start;
    }
  }

  // A tiny read joins the others started in the same turn, which go out
  // together once it ends.
  #tiny(offset: number, end: number): Segment {
    if (!this.#multiRange) return this.#request(offset, end, true);
    let read!: TinyRead;
    const data = new Promise<Uint8Array>((resolve, reject) => {
      read = { start: offset, end, resolve, reject };
    });
    if (!this.#tinyReads) {
      const batch: TinyRead[] = [];
      this.#tinyReads = batch;
      // The reads belong to the opening under way now, even if the reader
      // leaves before they go out.
      const signal = this.#cancel.signal;
      queueMicrotask(() => this.#sendTinyReads(batch, signal));
    }
    this.#tinyReads.push(read);
    return this.#forgetIfFailed(this.#keep(offset, end, data, true));
  }

  #sendTinyReads(batch: TinyRead[], signal: AbortSignal) {
    const reads = batch.sort((a, b) => a.start - b.start);
    if (this.#tinyReads === batch) this.#tinyReads = null;
    for (let i = 0; i < reads.length; i += MAX_RANGES) {
      const group = reads.slice(i, i + MAX_RANGES);
      // A request for several ranges that keeps failing leaves each to be
      // asked for on its own, unless the copy changed or the reader left.
      const parts =
        group.length > 1 && this.#multiRange
          ? withRetries(() => this.#fetchParts(group, signal), signal).catch(
              (error: unknown) => {
                if (isAbort(error) || error instanceof BookChangedError) {
                  throw error;
                }
                return [];
              },
            )
          : Promise.resolve([]);
      parts.then(
        (found) => {
          for (const read of group) {
            const part = found.find(
              (p) =>
                p.start <= read.start && p.start + p.data.length >= read.end,
            );
            // A part the answer left out is asked for on its own.
            const data = part
              ? Promise.resolve(
                  part.data.slice(
                    read.start - part.start,
                    read.end - part.start,
                  ),
                )
              : this.#fetchRange(read.start, read.end, signal);
            data.then(read.resolve, read.reject);
          }
        },
        (error: unknown) => {
          for (const read of group) read.reject(error);
        },
      );
    }
  }

  // One request for several ranges. Resolves the parts the server sent,
  // which may be none.
  async #fetchParts(reads: TinyRead[], signal: AbortSignal): Promise<Part[]> {
    const headers: Record<string, string> = {
      Range: `bytes=${reads.map(({ start, end }) => `${start}-${end - 1}`).join(",")}`,
    };
    if (this.#validator) headers["If-Range"] = this.#validator;
    return this.#withSlot(signal, async () => {
      const { response, drop, done } = await fetchDroppable(
        this.url,
        headers,
        signal,
      );
      try {
        if (response.status === 206) {
          this.#checkCopy(response, drop);
          const type = response.headers.get("Content-Type") ?? "";
          const boundary = /boundary=("?)([^";]+)\1/i.exec(type)?.[2];
          const body = new Uint8Array(await response.arrayBuffer());
          // A server may answer with one range that holds them all.
          const single = parseContentRange(
            response.headers.get("Content-Range"),
          );
          if (
            !boundary &&
            single &&
            body.length !== single.end - single.start
          ) {
            throw new BrokenAnswerError("The server sent part of the range.");
          }
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
        // The copy opened, sent whole: the server takes one range at a time.
        if (validatorOf(response) === this.#validator) {
          this.#multiRange = false;
          return [];
        }
        throw this.#markChanged();
      } finally {
        done();
      }
    });
  }

  #keep(
    start: number,
    end: number,
    data: Uint8Array | Promise<Uint8Array>,
    run: boolean,
  ): Segment {
    const segment: Segment = {
      start,
      end,
      data: Promise.resolve(data),
      settled: !(data instanceof Promise),
      run,
    };
    if (data instanceof Promise) {
      const settle = () => {
        segment.settled = true;
        this.#evict(segment);
      };
      data.then(settle, settle);
    }
    this.#segments.push(segment);
    this.#cached += end - start;
    this.#evict(segment);
    return segment;
  }

  // Drops the parts used longest ago until the cache is within its limit,
  // keeping fetches in flight, which have readers waiting, and `keep`, which
  // one is about to read.
  #evict(keep: Segment) {
    for (let i = 0; this.#cached > CACHE_LIMIT && i < this.#segments.length;) {
      const oldest = this.#segments[i];
      if (!oldest.settled || oldest === keep) {
        i++;
        continue;
      }
      this.#segments.splice(i, 1);
      this.#cached -= oldest.end - oldest.start;
    }
  }

  // Waits for one of MAX_REQUESTS to be free, then makes the request. A
  // request that ends hands its place to the next one waiting.
  async #withSlot<T>(signal: AbortSignal, request: () => Promise<T>) {
    if (this.#requests >= MAX_REQUESTS) {
      await new Promise<void>((resolve) => this.#queued.push(resolve));
    } else {
      this.#requests++;
    }
    try {
      if (signal.aborted) throw signal.reason;
      return await request();
    } finally {
      const next = this.#queued.shift();
      if (next) next();
      else this.#requests--;
    }
  }

  #fetchRange(
    start: number,
    end: number,
    signal = this.#cancel.signal,
  ): Promise<Uint8Array> {
    const headers: Record<string, string> = {
      Range: `bytes=${start}-${end - 1}`,
    };
    if (this.#validator) headers["If-Range"] = this.#validator;
    return withRetries(
      () =>
        this.#withSlot(signal, () =>
          this.#fetchOnce(headers, start, end, signal),
        ),
      signal,
    );
  }

  async #fetchOnce(
    headers: Record<string, string>,
    start: number,
    end: number,
    signal: AbortSignal,
  ): Promise<Uint8Array> {
    const { response, drop, done } = await fetchDroppable(
      this.url,
      headers,
      signal,
    );
    try {
      if (response.status === 206) {
        this.#checkCopy(response, drop);
        const range = parseContentRange(response.headers.get("Content-Range"));
        if (!range || range.start !== start || range.total !== this.size) {
          drop();
          throw this.#markChanged();
        }
        const data = new Uint8Array(await response.arrayBuffer());
        // Short, as a cut connection leaves it: worth another try.
        if (range.end < end || data.length !== range.end - start) {
          throw new BrokenAnswerError("The server sent part of the range.");
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

  // A part from another copy, as a proxy that drops If-Range would send,
  // says so in its validator.
  #checkCopy(response: Response, drop: () => void) {
    const validator = validatorOf(response);
    if (validator && this.#validator && validator !== this.#validator) {
      drop();
      throw this.#markChanged();
    }
  }

  #markChanged(): BookChangedError {
    if (!this.#changed) {
      this.#changed = new BookChangedError();
      const changed = this.#changed;
      queueMicrotask(() => this.#notify(changed));
    }
    return this.#changed;
  }
}
