import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ShishoAPIError } from "@/libraries/api";
import {
  BookChangedError,
  RangeFile,
  RangeNotSupportedError,
} from "@/libraries/rangeFile";
import { createRangeServer, type RangeServer } from "@/testing/rangeServer";

const URL = "/api/books/files/42/download";
const MB = 1024 * 1024;

// Bytes whose value says where they are, so a read from the wrong place shows.
const fileOf = (size: number) => {
  const bytes = new Uint8Array(size);
  for (let i = 0; i < size; i++) bytes[i] = (i * 7 + (i >> 8)) & 0xff;
  return bytes;
};

describe("RangeFile", () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;
  let server: RangeServer;
  let bytes: Uint8Array;

  const serve = (file: Uint8Array) => {
    bytes = file;
    server = createRangeServer(file);
    fetchSpy.mockImplementation(server.fetch);
  };

  beforeEach(() => {
    fetchSpy = vi.spyOn(globalThis, "fetch");
    serve(fileOf(10 * MB));
  });

  afterEach(() => {
    fetchSpy.mockRestore();
  });

  it("learns the size from the end of the file without downloading it", async () => {
    const file = await RangeFile.open(URL);

    expect(file.size).toBe(10 * MB);
    expect(server.requests).toHaveLength(1);
    expect(server.requests[0].range).toMatch(/^bytes=-\d+$/);
    expect(server.bytesSent()).toBeLessThan(MB);
  });

  it("reads the end of the file from what opening it fetched", async () => {
    const file = await RangeFile.open(URL);

    const tail = await file.read(10 * MB - 22, 22);

    expect(tail).toEqual(bytes.slice(10 * MB - 22));
    expect(server.requests).toHaveLength(1);
  });

  it("reads any part of the file, asking only for that part", async () => {
    const file = await RangeFile.open(URL);

    const middle = await file.read(3 * MB + 5, 1000);

    expect(middle).toEqual(bytes.slice(3 * MB + 5, 3 * MB + 1005));
    expect(server.requests).toHaveLength(2);
    expect(server.bytesSent()).toBeLessThan(2 * MB);
  });

  it("reads a large entry in a few requests, not one per chunk", async () => {
    const file = await RangeFile.open(URL);

    // zip.js reads an entry 64 KB at a time.
    const chunk = 64 * 1024;
    for (let at = MB; at < 6 * MB; at += chunk) {
      expect(await file.read(at, chunk)).toEqual(bytes.slice(at, at + chunk));
    }

    // 80 chunks.
    expect(server.requests.length).toBeLessThanOrEqual(10);
  });

  it("fetches through the end a caller names in one request", async () => {
    const file = await RangeFile.open(URL);

    // A zip entry: its header, then its data read 64 KB at a time.
    const end = 4 * MB;
    expect(await file.read(MB, 30, end)).toEqual(bytes.slice(MB, MB + 30));
    for (let at = MB + 30; at < end; at += 64 * 1024) {
      const len = Math.min(64 * 1024, end - at);
      expect(await file.read(at, len)).toEqual(bytes.slice(at, at + len));
    }

    expect(server.requests).toHaveLength(2);
    expect(server.requests[1].range).toBe(`bytes=${MB}-${end - 1}`);
  });

  it("fetches the rest of a run whose start an earlier read holds", async () => {
    const file = await RangeFile.open(URL);
    // A small entry, whose read ahead takes in the next entry's header.
    await file.read(MB, 100, MB + 100);

    const end = MB + 3 * MB;
    for (let at = MB + 100; at < end; at += 64 * 1024) {
      const len = Math.min(64 * 1024, end - at);
      expect(
        await file.read(at, len, at === MB + 100 ? end : undefined),
      ).toEqual(bytes.slice(at, at + len));
    }

    expect(server.requests).toHaveLength(3);
  });

  it("still reads ahead for a small entry", async () => {
    const file = await RangeFile.open(URL);

    await file.read(MB, 30, MB + 100);
    await file.read(MB + 100, 30, MB + 200);

    expect(server.requests).toHaveLength(2);
  });

  it("shares one request between reads of the same part", async () => {
    const file = await RangeFile.open(URL);

    const [a, b] = await Promise.all([
      file.read(2 * MB, 100),
      file.read(2 * MB + 50, 100),
    ]);

    expect(a).toEqual(bytes.slice(2 * MB, 2 * MB + 100));
    expect(b).toEqual(bytes.slice(2 * MB + 50, 2 * MB + 150));
    expect(server.requests).toHaveLength(2);
  });

  it("reads a file smaller than what opening asks for", async () => {
    serve(fileOf(1000));

    const file = await RangeFile.open(URL);

    expect(file.size).toBe(1000);
    expect(await file.read(0, 1000)).toEqual(bytes);
    expect(server.requests).toHaveLength(1);
  });

  it("waits for a Generated File being built rather than downloading it whole", async () => {
    server.buildFor(3);

    const opening = RangeFile.open(URL);
    await vi.advanceTimersByTimeAsync(10_000);
    const file = await opening;

    expect(file.size).toBe(10 * MB);
    expect(server.requests).toHaveLength(4);
  });

  it("stops waiting when the reader closes", async () => {
    server.buildFor(1000);
    const controller = new AbortController();

    const opening = RangeFile.open(URL, { signal: controller.signal });
    const settled = expect(opening).rejects.toThrow();
    await vi.advanceTimersByTimeAsync(1000);
    controller.abort();
    await settled;

    const asked = server.requests.length;
    await vi.advanceTimersByTimeAsync(10_000);
    expect(server.requests).toHaveLength(asked);
  });

  it("reports a server that ignores Range", async () => {
    fetchSpy.mockImplementation(
      async () =>
        new Response(new Uint8Array(100), {
          status: 200,
          headers: { "Accept-Ranges": "bytes" },
        }),
    );

    await expect(RangeFile.open(URL)).rejects.toBeInstanceOf(
      RangeNotSupportedError,
    );
  });

  it("surfaces the API's error", async () => {
    fetchSpy.mockImplementation(async () =>
      Response.json(
        { error: { code: "invalid_state", message: "Not supported" } },
        { status: 422 },
      ),
    );

    const error = await RangeFile.open(URL).catch((e: unknown) => e);

    expect(error).toBeInstanceOf(ShishoAPIError);
    expect((error as ShishoAPIError).status).toBe(422);
  });

  it("asks only for parts of the copy it opened", async () => {
    const file = await RangeFile.open(URL);

    await file.read(MB, 10);

    expect(server.requests[1].ifRange).toBe("Sun, 22 Nov 2026 19:36:13 GMT");
  });

  it("reports a book that changed on the server while it was open", async () => {
    const file = await RangeFile.open(URL);
    const changed = vi.fn();
    file.onChanged(changed);

    server.replace(fileOf(11 * MB), "Sun, 22 Nov 2026 20:00:00 GMT");

    await expect(file.read(MB, 10)).rejects.toBeInstanceOf(BookChangedError);
    expect(changed).toHaveBeenCalledWith(expect.any(BookChangedError));
  });

  it("reports a book being rebuilt while it was open as changed", async () => {
    const file = await RangeFile.open(URL);
    server.buildFor(1);

    await expect(file.read(MB, 10)).rejects.toBeInstanceOf(BookChangedError);
  });

  it("tries a dropped read again", async () => {
    const file = await RangeFile.open(URL);
    server.failFor(1);

    const reading = file.read(MB, 10);
    await vi.advanceTimersByTimeAsync(5000);

    expect(await reading).toEqual(bytes.slice(MB, MB + 10));
  });

  it("gives up on a read that keeps failing", async () => {
    const file = await RangeFile.open(URL);
    server.failFor(100);

    const reading = file.read(MB, 10);
    const settled = expect(reading).rejects.toBeInstanceOf(TypeError);
    await vi.advanceTimersByTimeAsync(30_000);
    await settled;
  });

  // foliate's MOBI reader reads the first four bytes of every resource
  // record to find two of them, across the whole file.
  describe("tiny reads at once", () => {
    const starts = Array.from({ length: 150 }, (_, i) => i * 60_000 + 7);

    it("asks for them together, in a few requests", async () => {
      const file = await RangeFile.open(URL);

      const read = await Promise.all(starts.map((at) => file.read(at, 4)));

      expect(read).toEqual(starts.map((at) => bytes.slice(at, at + 4)));
      // Opening, then the reads in groups.
      expect(server.requests.length).toBeLessThanOrEqual(4);
      expect(server.bytesSent()).toBeLessThan(MB);
    });

    it("asks only for the copy it opened", async () => {
      const file = await RangeFile.open(URL);
      server.replace(fileOf(10 * MB), "Sun, 22 Nov 2026 20:00:00 GMT");

      await expect(
        Promise.all(starts.map((at) => file.read(at, 4))),
      ).rejects.toBeInstanceOf(BookChangedError);
    });

    it("asks one at a time of a server that sends one range at a time", async () => {
      const file = await RangeFile.open(URL);
      server.singleRangeOnly();

      const some = starts.slice(0, 5);
      const read = await Promise.all(some.map((at) => file.read(at, 4)));

      expect(read).toEqual(some.map((at) => bytes.slice(at, at + 4)));
    });

    it("tries a dropped request again", async () => {
      const file = await RangeFile.open(URL);
      server.failFor(1);

      const reading = Promise.all(starts.map((at) => file.read(at, 4)));
      await vi.advanceTimersByTimeAsync(5000);

      expect(await reading).toEqual(
        starts.map((at) => bytes.slice(at, at + 4)),
      );
    });
  });

  describe("downloaded whole", () => {
    it("reads every part from the download, not the server", async () => {
      const file = await RangeFile.open(URL);

      expect(await file.downloadWhole()).toBe(true);
      const asked = server.requests.length;

      expect(await file.read(3 * MB + 5, 1000)).toEqual(
        bytes.slice(3 * MB + 5, 3 * MB + 1005),
      );
      expect(await file.read(9 * MB, 64 * 1024, 10 * MB)).toEqual(
        bytes.slice(9 * MB, 9 * MB + 64 * 1024),
      );
      expect(server.requests).toHaveLength(asked);
      expect(server.requests[server.requests.length - 1].range).toBeNull();
    });

    it("downloads once however often it's asked", async () => {
      const file = await RangeFile.open(URL);

      await Promise.all([file.downloadWhole(), file.downloadWhole()]);
      await file.downloadWhole();

      expect(server.requests.filter((r) => r.range === null)).toHaveLength(1);
    });

    it("keeps reading by part when the server's copy changed", async () => {
      const file = await RangeFile.open(URL);
      server.replace(fileOf(10 * MB), "Sun, 22 Nov 2026 20:00:00 GMT");

      expect(await file.downloadWhole()).toBe(false);
      // The copy opened is gone, which a read by part reports.
      await expect(file.read(MB, 10)).rejects.toBeInstanceOf(BookChangedError);
    });

    it("keeps reading by part when the download fails", async () => {
      const file = await RangeFile.open(URL);
      server.failFor(1);

      expect(await file.downloadWhole()).toBe(false);
      expect(await file.read(MB, 10)).toEqual(bytes.slice(MB, MB + 10));
    });

    it("stops downloading when the reader closes, and can start again", async () => {
      const file = await RangeFile.open(URL);
      const controller = new AbortController();
      controller.abort();

      expect(await file.downloadWhole({ signal: controller.signal })).toBe(
        false,
      );
      expect(await file.downloadWhole()).toBe(true);
    });
  });

  it("reads a part again after a failed read of it", async () => {
    const file = await RangeFile.open(URL);
    server.failFor(100);
    const reading = file.read(MB, 10);
    const settled = expect(reading).rejects.toThrow();
    await vi.advanceTimersByTimeAsync(30_000);
    await settled;

    server.failFor(0);

    expect(await file.read(MB, 10)).toEqual(bytes.slice(MB, MB + 10));
  });
});
